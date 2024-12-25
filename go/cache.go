package main

import (
	"sync"
	"time"

	"github.com/gammazero/deque"
	"github.com/oklog/ulid/v2"
)

type Notif struct {
	Ride         *Ride
	RideStatusID string
	RideStatus   string
}

type ChairStats struct {
	RideCount       int
	TotalEvaluation float64
}

type Location struct {
	Latitude  int
	Longitude int
}

type TotalDistance struct {
	TotalDistance int
	UpdatedAt     time.Time
}

type ChairSale struct {
	Sale      int
	UpdatedAt time.Time
}

var (
	latestRideStatusCache        = sync.Map{}
	latestRideCache              = sync.Map{}
	latestChairLocation          = sync.Map{}
	chairStatsCache              = sync.Map{}
	chairTotalDistanceCache      = sync.Map{}
	chairSpeedbyName             = sync.Map{}
	appNotifChan                 = sync.Map{}
	chairNotifChan               = sync.Map{}
	chairSaleCache               = sync.Map{}
	chairAccessTokenCache        = sync.Map{}
	appAccessTokenCache          = sync.Map{}
	ownerAccessTokenCache        = sync.Map{}
	ownerCache                   = sync.Map{}
	ownerChairRegisterTokenCache = sync.Map{}
	chairsOwnerIDCache           = sync.Map{}
	chairCache                   = sync.Map{}
	invCouponCountCache          = sync.Map{}
	unusedCouponsCache           = sync.Map{}
	rideDiscountCache            = sync.Map{}
	userCache                    = sync.Map{}
	userInvCache                 = sync.Map{}
	freeChairsCache              = NewFreeChairs()
)

func initCache() {
	latestRideStatusCache = sync.Map{}
	latestRideCache = sync.Map{}
	latestChairLocation = sync.Map{}
	chairStatsCache = sync.Map{}
	chairTotalDistanceCache = sync.Map{}
	chairSpeedbyName = sync.Map{}
	appNotifChan = sync.Map{}
	chairNotifChan = sync.Map{}
	chairSaleCache = sync.Map{}
	chairAccessTokenCache = sync.Map{}
	appAccessTokenCache = sync.Map{}
	ownerAccessTokenCache = sync.Map{}
	ownerCache = sync.Map{}
	ownerChairRegisterTokenCache = sync.Map{}
	chairsOwnerIDCache = sync.Map{}
	chairCache = sync.Map{}
	invCouponCountCache = sync.Map{}
	unusedCouponsCache = sync.Map{}
	rideDiscountCache = sync.Map{}
	userCache = sync.Map{}
	userInvCache = sync.Map{}
	freeChairsCache = NewFreeChairs()
}

func getLatestRideStatus(rideID string) (string, bool) {
	status, ok := latestRideStatusCache.Load(rideID)
	if !ok {
		return "", false
	}
	return status.(string), ok
}

func createRideStatus(rideID string, status string) {
	latestRideStatusCache.Store(rideID, status)
}

func processRideStatus(ride *Ride, status string) {
	createRideStatus(ride.ID, status)
	id := ulid.Make().String()
	notif := Notif{
		Ride:         ride,
		RideStatusID: id,
		RideStatus:   status,
	}
	publishAppChan(ride.UserID, notif)
	if ride.ChairID.Valid {
		publishChairChan(ride.ChairID.String, notif)
	}
	if status == "COMPLETED" {
		createChairSaleCache(ride)
	}
}

func publishAppChan(userID string, notif Notif) {
	appChan, ok := appNotifChan.Load(userID)
	if !ok {
		appNotifChan.Store(userID, make(chan Notif, 5))
		appChan, _ = appNotifChan.Load(userID)
	}
	appChan.(chan Notif) <- notif
}

func publishChairChan(chairID string, notif Notif) {
	chairChan, ok := chairNotifChan.Load(chairID)
	if !ok {
		chairNotifChan.Store(chairID, make(chan Notif, 5))
		chairChan, _ = chairNotifChan.Load(chairID)
	}
	chairChan.(chan Notif) <- notif
}

func createChairSaleCache(ride *Ride) {
	chairSales := []ChairSale{}
	if salesAny, ok := chairSaleCache.Load(ride.ChairID.String); ok {
		chairSales = salesAny.([]ChairSale)
	}
	chairSales = append(chairSales, ChairSale{
		Sale:      calculateSale(*ride),
		UpdatedAt: ride.UpdatedAt,
	})
	chairSaleCache.Store(ride.ChairID.String, chairSales)
}

func createChairLocation(chairID string, chairLocation ChairLocation) {
	latestChairLocation.Store(chairID, chairLocation)
}

func getLatestChairLocationChacke(chairID string) (ChairLocation, bool) {
	latest, ok := latestChairLocation.Load(chairID)
	if !ok {
		return ChairLocation{}, false
	}
	return latest.(ChairLocation), ok
}

func getChairTotalDistanceCache(chairID string) (TotalDistance, bool) {
	totalDistance, ok := chairTotalDistanceCache.Load(chairID)
	if !ok {
		return TotalDistance{}, false
	}
	return totalDistance.(TotalDistance), ok
}

func createChairTotalDistanceCache(chairID string, distance int, now time.Time) {
	current, _ := getChairTotalDistanceCache(chairID)
	chairTotalDistanceCache.Store(chairID, TotalDistance{
		TotalDistance: current.TotalDistance + distance,
		UpdatedAt:     now,
	})
}

func getLatestChairLocation(chairID string) (ChairLocation, bool) {
	loc, ok := latestChairLocation.Load(chairID)
	if !ok {
		return ChairLocation{}, false
	}
	return loc.(ChairLocation), ok
}

func getChairStatsCache(chairID string) (ChairStats, bool) {
	stats, ok := chairStatsCache.Load(chairID)
	if !ok {
		return ChairStats{}, false
	}
	return stats.(ChairStats), ok
}

func addChairStatsCache(chairID string, evaluation int) {
	stats, ok := getChairStatsCache(chairID)
	if !ok {
		chairStatsCache.Store(chairID, ChairStats{
			RideCount:       1,
			TotalEvaluation: float64(evaluation),
		})
		return
	}
	stats.RideCount++
	stats.TotalEvaluation += float64(evaluation)
	chairStatsCache.Store(chairID, stats)
}

func getChairAccessToken(token string) (Chair, bool) {
	chair, ok := chairAccessTokenCache.Load(token)
	return chair.(Chair), ok
}

func createChairAccessToken(token string, chair Chair) {
	chairAccessTokenCache.Store(token, chair)
}

func getAppAccessToken(token string) (User, bool) {
	user, ok := appAccessTokenCache.Load(token)
	return user.(User), ok
}

func createAppAccessToken(token string, user User) {
	appAccessTokenCache.Store(token, user)
}

func getOwnerAccessToken(token string) (Owner, bool) {
	owner, ok := ownerAccessTokenCache.Load(token)
	return owner.(Owner), ok
}

func createOwnerAccessToken(token string, owner Owner) {
	ownerAccessTokenCache.Store(token, owner)
}

type FreeChairs struct {
	cache map[string]Chair
	mu    sync.Mutex
}

func NewFreeChairs() *FreeChairs {
	return &FreeChairs{
		cache: map[string]Chair{},
		mu:    sync.Mutex{},
	}
}
func (f *FreeChairs) Lock() {
	f.mu.Lock()
}

func (f *FreeChairs) Unlock() {
	f.mu.Unlock()
}

func (f *FreeChairs) List() []Chair {
	chairs := []Chair{}
	for _, v := range f.cache {
		chairs = append(chairs, v)
	}
	return chairs
}

func (f *FreeChairs) Add(chair Chair) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cache[chair.ID] = chair
}

func (f *FreeChairs) BulkRemove(chairIDs []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, chairID := range chairIDs {
		delete(f.cache, chairID)
	}
}

func (f *FreeChairs) Remove(chairID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.cache, chairID)
}

func getOwnerCache(ownerID string) (Owner, bool) {
	owner, ok := ownerCache.Load(ownerID)
	return owner.(Owner), ok
}

func createOwnerCache(ownerID string, owner Owner) {
	ownerCache.Store(ownerID, owner)
}

func getOwnerChairRegisterTokenCache(chairRegisterToken string) (Owner, bool) {
	owner, ok := ownerChairRegisterTokenCache.Load(chairRegisterToken)
	return owner.(Owner), ok
}

func createOwnerChairRegisterTokenCache(chairRegisterToken string, owner Owner) {
	ownerChairRegisterTokenCache.Store(chairRegisterToken, owner)
}

func getChairsOwnerIDCache(ownerID string) ([]Chair, bool) {
	chairs, ok := chairsOwnerIDCache.Load(ownerID)
	if !ok {
		return []Chair{}, false
	}
	return chairs.([]Chair), ok
}

func createChairsOwnerIDCache(ownerID string, chair Chair) {
	chairs := []Chair{}
	tmp, ok := getChairsOwnerIDCache(ownerID)
	if ok {
		chairs = tmp
	}
	chairs = append(chairs, chair)
	chairsOwnerIDCache.Store(ownerID, chairs)
}

func getChairCache(chairID string) (Chair, bool) {
	chair, ok := chairCache.Load(chairID)
	return chair.(Chair), ok
}

func createChairCache(chairID string, chair Chair) {
	chairCache.Store(chairID, chair)
}

func getInvCouponCountCache(code string) (int, bool) {
	count, ok := invCouponCountCache.Load(code)
	if !ok {
		return 0, false
	}
	return count.(int), ok
}

func incInvCouponCountCache(code string) {
	count := 0
	if current, ok := getInvCouponCountCache(code); ok {
		count = current
	}
	invCouponCountCache.Store(code, count+1)
}

type UnusedCouponAmount struct {
	cache deque.Deque[int]
	mu    sync.Mutex
}

func NewUnusedCouponAmount() *UnusedCouponAmount {
	return &UnusedCouponAmount{
		cache: deque.Deque[int]{},
		mu:    sync.Mutex{},
	}
}

func (u *UnusedCouponAmount) Len() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.cache.Len()
}

func (u *UnusedCouponAmount) Add(amount int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.cache.PushBack(amount)
}

func (u *UnusedCouponAmount) Front() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.cache.Front()
}

func (u *UnusedCouponAmount) Remove() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.cache.PopFront()
}

func addUnusedCoupon(userID string, amount int) {
	unusedCouponAmount := NewUnusedCouponAmount()
	if tmp, ok := unusedCouponsCache.Load(userID); ok {
		unusedCouponAmount = tmp.(*UnusedCouponAmount)
	}
	unusedCouponAmount.Add(amount)
	unusedCouponsCache.Store(userID, unusedCouponAmount)
}

func getUnusedCoupon(userID string) (int, bool) {
	unusedCouponAmount := NewUnusedCouponAmount()
	tmp, ok := unusedCouponsCache.Load(userID)
	if !ok {
		return 0, false
	}
	unusedCouponAmount = tmp.(*UnusedCouponAmount)
	if unusedCouponAmount.Len() == 0 {
		return 0, false
	}
	return unusedCouponAmount.Front(), true
}

func useUnusedCoupon(userID string) int {
	unusedCouponAmount := NewUnusedCouponAmount()
	if tmp, ok := unusedCouponsCache.Load(userID); ok {
		unusedCouponAmount = tmp.(*UnusedCouponAmount)
	}
	return unusedCouponAmount.Remove()
}

func getRideDiscountCache(rideID string) (int, bool) {
	discount, ok := rideDiscountCache.Load(rideID)
	if !ok {
		return 0, false
	}
	return discount.(int), ok
}

func createRideDiscountCache(rideID string, discount int) {
	rideDiscountCache.Store(rideID, discount)
}

func getUserCache(userID string) (User, bool) {
	user, ok := userCache.Load(userID)
	if !ok {
		return User{}, false
	}
	return user.(User), ok
}

func createUserCache(userID string, user User) {
	userCache.Store(userID, user)
}

func getUserInvCache(code string) (User, bool) {
	user, ok := userInvCache.Load(code)
	if !ok {
		return User{}, false
	}
	return user.(User), ok
}

func createUserInvCache(code string, user User) {
	userInvCache.Store(code, user)
}
