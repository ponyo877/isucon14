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
	latestRideStatus        = sync.Map{}
	latestRide              = sync.Map{}
	latestChairLocation     = sync.Map{}
	chairStats              = sync.Map{}
	chairTotalDistance      = sync.Map{}
	chairSpeedbyName        = sync.Map{}
	appNotifChan            = sync.Map{}
	chairNotifChan          = sync.Map{}
	chairSale               = sync.Map{}
	chairAccessToken        = sync.Map{}
	appAccessToken          = sync.Map{}
	ownerAccessToken        = sync.Map{}
	ownerCache              = sync.Map{}
	ownerChairRegisterToken = sync.Map{}
	chairsOwnerID           = sync.Map{}
	chairCache              = sync.Map{}
	invCouponCount          = sync.Map{}
	unusedCoupons           = sync.Map{}
	rideDiscount            = sync.Map{}
	userCache               = sync.Map{}
	userInv                 = sync.Map{}
	rideCache               = sync.Map{}
	paymentToken            = sync.Map{}
	userRideStatus          = sync.Map{}
	rideIDsUserID           = sync.Map{}
	freeChairs              = NewFreeChairs()
	waitingRides            = NewWaitingRides()
)

func initCache() {
	latestRideStatus = sync.Map{}
	latestRide = sync.Map{}
	latestChairLocation = sync.Map{}
	chairStats = sync.Map{}
	chairTotalDistance = sync.Map{}
	chairSpeedbyName = sync.Map{}
	appNotifChan = sync.Map{}
	chairNotifChan = sync.Map{}
	chairSale = sync.Map{}
	chairAccessToken = sync.Map{}
	appAccessToken = sync.Map{}
	ownerAccessToken = sync.Map{}
	ownerCache = sync.Map{}
	ownerChairRegisterToken = sync.Map{}
	chairsOwnerID = sync.Map{}
	chairCache = sync.Map{}
	invCouponCount = sync.Map{}
	unusedCoupons = sync.Map{}
	rideDiscount = sync.Map{}
	userCache = sync.Map{}
	userInv = sync.Map{}
	rideCache = sync.Map{}
	paymentToken = sync.Map{}
	userRideStatus = sync.Map{}
	rideIDsUserID = sync.Map{}
	freeChairs = NewFreeChairs()
	waitingRides = NewWaitingRides()
}

func getLatestRideStatus(rideID string) (string, bool) {
	status, ok := latestRideStatus.Load(rideID)
	if !ok {
		return "", false
	}
	return status.(string), ok
}

func createLatestRideStatus(rideID string, status string) {
	latestRideStatus.Store(rideID, status)
}

func getLatestRide(chairID string) (*Ride, bool) {
	ride, ok := latestRide.Load(chairID)
	if !ok {
		return nil, false
	}
	return ride.(*Ride), ok
}

func createLatestRide(chairID string, ride *Ride) {
	latestRide.Store(chairID, ride)
}

func deleteLatestRide(chairID string) {
	latestRide.Delete(chairID)
}

func processRideStatus(ride *Ride, status string) {
	createLatestRideStatus(ride.ID, status)
	id := ulid.Make().String()
	notif := &Notif{
		Ride:         ride,
		RideStatusID: id,
		RideStatus:   status,
	}
	publishAppChan(ride.UserID, notif)
	if ride.ChairID.Valid {
		publishChairChan(ride.ChairID.String, notif)
	}
	if status == "COMPLETED" {
		createChairSale(ride)
		createUserRideStatus(ride.UserID, true)
	}
}

func getAppChan(userID string) chan *Notif {
	appChan, ok := appNotifChan.Load(userID)
	if !ok {
		appNotifChan.Store(userID, make(chan *Notif, 5))
		appChan, _ = appNotifChan.Load(userID)
	}
	return appChan.(chan *Notif)
}

func getChairChan(chairID string) chan *Notif {
	chairChan, ok := chairNotifChan.Load(chairID)
	if !ok {
		chairNotifChan.Store(chairID, make(chan *Notif, 5))
		chairChan, _ = chairNotifChan.Load(chairID)
	}
	return chairChan.(chan *Notif)
}

func publishAppChan(userID string, notif *Notif) {
	getAppChan(userID) <- notif
}

func publishChairChan(chairID string, notif *Notif) {
	getChairChan(chairID) <- notif
}

func getChairSale(chairID string) ([]*ChairSale, bool) {
	sales, ok := chairSale.Load(chairID)
	if !ok {
		return []*ChairSale{}, false
	}
	return sales.([]*ChairSale), ok
}

func createChairSale(ride *Ride) {
	chairSales, _ := getChairSale(ride.ChairID.String)
	chairSales = append(chairSales, &ChairSale{
		Sale:      calculateSale(*ride),
		UpdatedAt: ride.UpdatedAt,
	})
	chairSale.Store(ride.ChairID.String, chairSales)
}

func createChairLocation(chairID string, chairLocation *ChairLocation) {
	latestChairLocation.Store(chairID, chairLocation)
}

func getLatestChairLocation(chairID string) (*ChairLocation, bool) {
	latest, ok := latestChairLocation.Load(chairID)
	if !ok {
		return nil, false
	}
	return latest.(*ChairLocation), ok
}

func getChairTotalDistance(chairID string) (*TotalDistance, bool) {
	totalDistance, ok := chairTotalDistance.Load(chairID)
	if !ok {
		return nil, false
	}
	return totalDistance.(*TotalDistance), ok
}

func createChairTotalDistance(chairID string, distance int, now time.Time) {
	current := &TotalDistance{}
	if tmp, ok := getChairTotalDistance(chairID); ok {
		current = tmp
	}
	chairTotalDistance.Store(chairID, &TotalDistance{
		TotalDistance: current.TotalDistance + distance,
		UpdatedAt:     now,
	})
}

func getChairSpeedbyName(model string) (int, bool) {
	speed, ok := chairSpeedbyName.Load(model)
	if !ok {
		return 0, false
	}
	return speed.(int), ok
}

func createChairSpeedbyName(model string, speed int) {
	chairSpeedbyName.Store(model, speed)
}

func getChairStatsCache(chairID string) (*ChairStats, bool) {
	stats, ok := chairStats.Load(chairID)
	if !ok {
		return nil, false
	}
	return stats.(*ChairStats), ok
}

func addChairStats(chairID string, evaluation int) {
	stats, ok := getChairStatsCache(chairID)
	if !ok {
		chairStats.Store(chairID, &ChairStats{
			RideCount:       1,
			TotalEvaluation: float64(evaluation),
		})
		return
	}
	stats.RideCount++
	stats.TotalEvaluation += float64(evaluation)
	chairStats.Store(chairID, stats)
}

func getChairAccessToken(token string) (*Chair, bool) {
	chair, ok := chairAccessToken.Load(token)
	return chair.(*Chair), ok
}

func createChairAccessToken(token string, chair *Chair) {
	chairAccessToken.Store(token, chair)
}

func getAppAccessToken(token string) (*User, bool) {
	user, ok := appAccessToken.Load(token)
	return user.(*User), ok
}

func createAppAccessToken(token string, user *User) {
	appAccessToken.Store(token, user)
}

func getOwnerAccessToken(token string) (*Owner, bool) {
	owner, ok := ownerAccessToken.Load(token)
	return owner.(*Owner), ok
}

func createOwnerAccessToken(token string, owner *Owner) {
	ownerAccessToken.Store(token, owner)
}

type FreeChairs struct {
	cache map[string]*Chair
	mu    sync.Mutex
}

func NewFreeChairs() *FreeChairs {
	return &FreeChairs{
		cache: map[string]*Chair{},
		mu:    sync.Mutex{},
	}
}
func (f *FreeChairs) Lock() {
	f.mu.Lock()
}

func (f *FreeChairs) Unlock() {
	f.mu.Unlock()
}

func (f *FreeChairs) List() []*Chair {
	chairs := []*Chair{}
	for _, v := range f.cache {
		chairs = append(chairs, v)
	}
	return chairs
}

func (f *FreeChairs) Add(chair *Chair) {
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

func getOwner(ownerID string) (*Owner, bool) {
	owner, ok := ownerCache.Load(ownerID)
	return owner.(*Owner), ok
}

func createOwner(ownerID string, owner *Owner) {
	ownerCache.Store(ownerID, owner)
}

func getOwnerChairRegisterToken(chairRegisterToken string) (*Owner, bool) {
	owner, ok := ownerChairRegisterToken.Load(chairRegisterToken)
	return owner.(*Owner), ok
}

func createOwnerChairRegisterToken(chairRegisterToken string, owner *Owner) {
	ownerChairRegisterToken.Store(chairRegisterToken, owner)
}

func getChairsOwnerID(ownerID string) ([]*Chair, bool) {
	chairs, ok := chairsOwnerID.Load(ownerID)
	if !ok {
		return []*Chair{}, false
	}
	return chairs.([]*Chair), ok
}

func createChairsOwnerID(ownerID string, chair *Chair) {
	chairs := []*Chair{}
	tmp, ok := getChairsOwnerID(ownerID)
	if ok {
		chairs = tmp
	}
	chairs = append(chairs, chair)
	chairsOwnerID.Store(ownerID, chairs)
}

func getChair(chairID string) (*Chair, bool) {
	chair, ok := chairCache.Load(chairID)
	return chair.(*Chair), ok
}

func createChair(chairID string, chair *Chair) {
	chairCache.Store(chairID, chair)
}

func getInvCouponCount(code string) (int, bool) {
	count, ok := invCouponCount.Load(code)
	if !ok {
		return 0, false
	}
	return count.(int), ok
}

func incInvCouponCount(code string) {
	count := 0
	if current, ok := getInvCouponCount(code); ok {
		count = current
	}
	invCouponCount.Store(code, count+1)
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
	if tmp, ok := unusedCoupons.Load(userID); ok {
		unusedCouponAmount = tmp.(*UnusedCouponAmount)
	}
	unusedCouponAmount.Add(amount)
	unusedCoupons.Store(userID, unusedCouponAmount)
}

func getUnusedCoupon(userID string) (int, bool) {
	unusedCouponAmount := NewUnusedCouponAmount()
	tmp, ok := unusedCoupons.Load(userID)
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
	if tmp, ok := unusedCoupons.Load(userID); ok {
		unusedCouponAmount = tmp.(*UnusedCouponAmount)
	}
	return unusedCouponAmount.Remove()
}

func getRideDiscount(rideID string) (int, bool) {
	discount, ok := rideDiscount.Load(rideID)
	if !ok {
		return 0, false
	}
	return discount.(int), ok
}

func createRideDiscount(rideID string, discount int) {
	rideDiscount.Store(rideID, discount)
}

func getUser(userID string) (*User, bool) {
	user, ok := userCache.Load(userID)
	if !ok {
		return nil, false
	}
	return user.(*User), ok
}

func createUser(userID string, user *User) {
	userCache.Store(userID, user)
}

func getUserInv(code string) (*User, bool) {
	user, ok := userInv.Load(code)
	if !ok {
		return nil, false
	}
	return user.(*User), ok
}

func createUserInv(code string, user *User) {
	userInv.Store(code, user)
}

func getRide(rideID string) (*Ride, bool) {
	ride, ok := rideCache.Load(rideID)
	if !ok {
		return nil, false
	}
	return ride.(*Ride), ok
}

func createRide(rideID string, ride *Ride) {
	rideCache.Store(rideID, ride)
}

func getPaymentToken(userID string) (string, bool) {
	token, ok := paymentToken.Load(userID)
	if !ok {
		return "", false
	}
	return token.(string), ok
}

func createPaymentToken(userID string, token string) {
	paymentToken.Store(userID, token)
}

func getUserRideStatus(userID string) (bool, bool) {
	isFree, ok := userRideStatus.Load(userID)
	if !ok {
		return false, false
	}
	return isFree.(bool), ok
}

func createUserRideStatus(userID string, isFree bool) {
	userRideStatus.Store(userID, isFree)
}

func listRideIDsUserID(userID string) ([]string, bool) {
	rideIDs, ok := rideIDsUserID.Load(userID)
	if !ok {
		return []string{}, false
	}
	return rideIDs.([]string), ok
}

func addRideIDsUserID(userID string, rideID string) {
	rideIDs := []string{}
	tmp, ok := listRideIDsUserID(userID)
	if ok {
		rideIDs = tmp
	}
	rideIDs = append(rideIDs, rideID)
	rideIDsUserID.Store(userID, rideIDs)
}

type WaitingRides struct {
	cache map[string]*Ride
	mu    sync.Mutex
}

func NewWaitingRides() *WaitingRides {
	return &WaitingRides{
		cache: map[string]*Ride{},
		mu:    sync.Mutex{},
	}
}
func (w *WaitingRides) Lock() {
	w.mu.Lock()
}

func (w *WaitingRides) Unlock() {
	w.mu.Unlock()
}

func (w *WaitingRides) List() []*Ride {
	w.mu.Lock()
	defer w.mu.Unlock()
	rides := []*Ride{}
	for _, v := range w.cache {
		rides = append(rides, v)
	}
	return rides
}

func (w *WaitingRides) Add(ride *Ride) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.cache[ride.ID] = ride
}

func (w *WaitingRides) BulkRemove(rideIDs []string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, rideID := range rideIDs {
		delete(w.cache, rideID)
	}
}

func (w *WaitingRides) Remove(rideID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.cache, rideID)
}
