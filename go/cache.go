package main

import (
	"context"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/oklog/ulid/v2"
)

type Notif struct {
	Ride         *Ride
	RideStatusID string
	RideStatus   string
}

type ChairStats struct {
	RideCount       int
	TotalEvaluation int
}

type Location struct {
	Latitude  int
	Longitude int
}

type TotalDistance struct {
	TotalDistance int
	UpdatedAt     time.Time
}

var (
	latestRideStatusCache   = sync.Map{}
	latestRideCache         = sync.Map{}
	latestChairLocation     = sync.Map{}
	chairStatsCache         = sync.Map{}
	chairTotalDistanceCache = sync.Map{}
	chairSpeedbyName        = sync.Map{}
	appNotifChan            = sync.Map{}
	chairNotifChan          = sync.Map{}
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
}

func getLatestRideStatus(ctx context.Context, tx executableGet, rideID string) (string, error) {
	if status, ok := latestRideStatusCache.Load(rideID); ok {
		return status.(string), nil
	}
	status := ""
	if err := tx.GetContext(ctx, &status, `SELECT status FROM ride_statuses WHERE ride_id = ? ORDER BY created_at DESC LIMIT 1`, rideID); err != nil {
		return "", err
	}
	return status, nil
}

func createRideStatus(ctx context.Context, tx *sqlx.Tx, ride *Ride, status string) (func(), error) {
	id := ulid.Make().String()
	_, err := tx.ExecContext(
		ctx,
		`INSERT INTO ride_statuses (id, ride_id, status) VALUES (?, ?, ?)`,
		id, ride.ID, status,
	)
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
	}

	return lazyDo, err
}

func createRideStatusDB(ctx context.Context, db *sqlx.DB, ride *Ride, status string) (func(), error) {
	id := ulid.Make().String()
	_, err := db.ExecContext(
		ctx,
		`INSERT INTO ride_statuses (id, ride_id, status) VALUES (?, ?, ?)`,
		id, ride.ID, status,
	)
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
	}

	return lazyDo, err
}

func getLatestRide(ctx context.Context, tx *sqlx.Tx, chairID string) (Ride, error) {
	if ride, ok := latestRideCache.Load(chairID); ok {
		return ride.(Ride), nil
	}
	ride := &Ride{}
	if err := tx.GetContext(ctx, ride, `SELECT * FROM rides WHERE chair_id = ? ORDER BY updated_at DESC LIMIT 1`, chairID); err != nil {
		return Ride{}, err
	}
	latestRideCache.Store(chairID, *ride)
	return *ride, nil
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
	stats := getChairStatsCache(chairID)
	stats.RideCount++
	stats.TotalEvaluation += evaluation
	chairStatsCache.Store(chairID, stats)
}
