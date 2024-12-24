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

func getLatestRideStatus(rideID string) string {
	status, _ := latestRideStatusCache.Load(rideID)
	return status.(string)
}

func createRideStatus(ride *Ride, status string) (func(), error) {
	id := ulid.Make().String()
	// _, err := tx.ExecContext(
	// 	ctx,
	// 	`INSERT INTO ride_statuses (id, ride_id, status) VALUES (?, ?, ?)`,
	// 	id, ride.ID, status,
	// )
	lazyDo := func() {
		latestRideStatusCache.Store(ride.ID, status)
		notif := Notif{
			Ride:         ride,
			RideStatusID: id,
			RideStatus:   status,
		}
		appChan, ok := appNotifChan.Load(ride.UserID)
		if !ok {
			appNotifChan.Store(ride.UserID, make(chan Notif, 5))
			appChan, _ = appNotifChan.Load(ride.UserID)
		}
		appChan.(chan Notif) <- notif
		if ride.ChairID.Valid {
			chairChan, ok := chairNotifChan.Load(ride.ChairID.String)
			if !ok {
				chairNotifChan.Store(ride.ChairID.String, make(chan Notif, 5))
				chairChan, _ = chairNotifChan.Load(ride.ChairID.String)
			}
			chairChan.(chan Notif) <- notif
		}
		if status == "COMPLETED" {
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
	}

	return lazyDo, nil
}

func createChairLocation(id, chairID string, latitude, longitude int, now time.Time) (func(), error) {
	lazyDo := func() {
		chairLoctionAny, ok := latestChairLocation.Load(chairID)
		if ok {
			before := chairLoctionAny.(ChairLocation)
			distance := calculateDistance(before.Latitude, before.Longitude, latitude, longitude)
			currentTotalDistance := 0
			if currentAny, ok := chairTotalDistanceCache.Load(chairID); ok {
				current := currentAny.(TotalDistance)
				currentTotalDistance = current.TotalDistance
			}
			chairTotalDistanceCache.Store(chairID, TotalDistance{
				TotalDistance: currentTotalDistance + distance,
				UpdatedAt:     now,
			})
		}
		latestChairLocation.Store(chairID, ChairLocation{
			ID:        id,
			ChairID:   chairID,
			Latitude:  latitude,
			Longitude: longitude,
			CreatedAt: now,
		})
	}
	return lazyDo, nil
}

func getLatestChairLocation(chairID string) ChairLocation {
	if loc, ok := latestChairLocation.Load(chairID); ok {
		return loc.(ChairLocation)
	}
	return ChairLocation{}
}

func getChairStatsCache(chairID string) ChairStats {
	if stats, ok := chairStatsCache.Load(chairID); ok {
		return stats.(ChairStats)
	}
	return ChairStats{}
}

func addChairStatsCache(chairID string, evaluation int) {
	if statsAny, ok := chairStatsCache.Load(chairID); ok {
		stats := statsAny.(ChairStats)
		stats.RideCount++
		stats.TotalEvaluation += float64(evaluation)
		chairStatsCache.Store(chairID, stats)
		return
	}
	chairStatsCache.Store(chairID, ChairStats{
		RideCount:       1,
		TotalEvaluation: float64(evaluation),
	})
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

func (f *FreeChairs) List() []Chair {
	f.mu.Lock()
	defer f.mu.Unlock()
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
