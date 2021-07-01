package memory

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/coocood/freecache"
	pb "github.com/envoyproxy/go-control-plane/envoy/service/ratelimit/v3"
	"github.com/envoyproxy/ratelimit/src/config"
	"github.com/envoyproxy/ratelimit/src/limiter"
	"github.com/envoyproxy/ratelimit/src/settings"
	"github.com/envoyproxy/ratelimit/src/stats"
	"github.com/envoyproxy/ratelimit/src/utils"
	gostats "github.com/lyft/gostats"
	"github.com/patrickmn/go-cache"
	logger "github.com/sirupsen/logrus"
)

// MemoryError that may be raised during config parsing.
type MemoryError string

func (e MemoryError) Error() string {
	return string(e)
}

type rateLimitMemoryImpl struct {
	memoryCache                *cache.Cache
	timeSource                 utils.TimeSource
	jitterRand                 *rand.Rand
	expirationJitterMaxSeconds int64
	localCache                 *freecache.Cache
	waitGroup                  sync.WaitGroup
	nearLimitRatio             float32
	baseRateLimiter            *limiter.BaseRateLimiter
}

func (this *rateLimitMemoryImpl) DoLimit(ctx context.Context,
	request *pb.RateLimitRequest,
	limits []*config.RateLimit) []*pb.RateLimitResponse_DescriptorStatus {
	logger.Debugf("starting cache lookup")

	// request.HitsAddend could be 0 (default value) if not specified by the caller in the Ratelimit request.
	hitsAddend := utils.Max(1, request.HitsAddend)

	// First build a list of all cache keys that we are actually going to hit.
	cacheKeys := this.baseRateLimiter.GenerateCacheKeys(request, limits, hitsAddend)

	isOverLimitWithLocalCache := make([]bool, len(request.Descriptors))

	results := make([]uint32, len(request.Descriptors))

	for i, cacheKey := range cacheKeys {
		if cacheKey.Key == "" {
			continue
		}

		// Check if key is over the limit in local cache.
		if this.baseRateLimiter.IsOverLimitWithLocalCache(cacheKey.Key) {
			isOverLimitWithLocalCache[i] = true
			logger.Debugf("cache key is over the limit: %s", cacheKey.Key)
			continue
		}

		logger.Debugf("looking up cache key: %s", cacheKey.Key)
		expirationSeconds := utils.UnitToDivider(limits[i].Limit.Unit)
		if this.expirationJitterMaxSeconds > 0 {
			expirationSeconds += this.jitterRand.Int63n(this.expirationJitterMaxSeconds)
		}
		results[i] = this.increase(cacheKey.Key, hitsAddend, expirationSeconds)
	}

	// Now fetch from memory
	responseDescriptorStatuses := make([]*pb.RateLimitResponse_DescriptorStatus,
		len(request.Descriptors))

	for i, cacheKey := range cacheKeys {
		limitAfterIncrease := results[i]
		limitBeforeIncrease := limitAfterIncrease - hitsAddend

		limitInfo := limiter.NewRateLimitInfo(limits[i], limitBeforeIncrease, limitAfterIncrease, 0, 0)

		responseDescriptorStatuses[i] = this.baseRateLimiter.GetResponseDescriptorStatus(cacheKey.Key,
			limitInfo, isOverLimitWithLocalCache[i], hitsAddend)
	}
	return responseDescriptorStatuses
}

func (this *rateLimitMemoryImpl) increase(key string, hitsAddend uint32, expirationSeconds int64) uint32 {
	ret, err := this.memoryCache.IncrementUint32(key, hitsAddend)
	if err != nil {
		_, found := this.memoryCache.Get(key)
		if !found {
			// Need to add instead of increment.
			this.memoryCache.Set(key, hitsAddend, time.Second*time.Duration(expirationSeconds))
			ret = hitsAddend
		} else {
			panic(MemoryError(fmt.Sprintf("failed to increment: %v", err)))
		}
	}
	return ret
}

func (this *rateLimitMemoryImpl) Flush() {

}

func newMemoryCacheFromSettings(s settings.Settings) *cache.Cache {
	if s.MemoryCacheDefaultExpiration == 0 || s.MemoryCacheCleanupInterval == 0 {
		panic(MemoryError("Both MEMORY_CACHE_DEFAULT_EXPIRATION and MEMORY_CACHE_CLEANUP_INTERVAL can not be zero"))
	}
	logger.Debugf("Usng MEMORY_CACHE_DEFAULT_EXPIRATION:: %v", s.MemoryCacheDefaultExpiration)
	logger.Debugf("Usng MEMORY_CACHE_CLEANUP_INTERVAL:: %v", s.MemoryCacheCleanupInterval)
	// Create a cache with a default expiration time of MEMORY_CACHE_DEFAULT_EXPIRATION, and which
	// purges expired items every MEMORY_CACHE_CLEANUP_INTERVAL
	memoryCache := cache.New(s.MemoryCacheDefaultExpiration, s.MemoryCacheCleanupInterval)
	return memoryCache
}

func NewRateLimitCacheImpl(memoryCache *cache.Cache, timeSource utils.TimeSource, jitterRand *rand.Rand,
	expirationJitterMaxSeconds int64, localCache *freecache.Cache, statsManager stats.Manager, nearLimitRatio float32, cacheKeyPrefix string) limiter.RateLimitCache {
	return &rateLimitMemoryImpl{
		memoryCache:                memoryCache,
		timeSource:                 timeSource,
		jitterRand:                 jitterRand,
		expirationJitterMaxSeconds: expirationJitterMaxSeconds,
		localCache:                 localCache,
		nearLimitRatio:             nearLimitRatio,
		baseRateLimiter:            limiter.NewBaseRateLimit(timeSource, jitterRand, expirationJitterMaxSeconds, localCache, nearLimitRatio, cacheKeyPrefix, statsManager),
	}
}

func NewRateLimiterCacheImplFromSettings(s settings.Settings, timeSource utils.TimeSource, jitterRand *rand.Rand,
	localCache *freecache.Cache, scope gostats.Scope, statsManager stats.Manager) limiter.RateLimitCache {
	return NewRateLimitCacheImpl(
		newMemoryCacheFromSettings(s),
		timeSource,
		jitterRand,
		s.ExpirationJitterMaxSeconds,
		localCache,
		statsManager,
		s.NearLimitRatio,
		s.CacheKeyPrefix,
	)
}
