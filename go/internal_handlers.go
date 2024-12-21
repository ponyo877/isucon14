package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"sync"

	mcf "github.com/isucon/isucon14/webapp/go/mincostflow"
)

var mu sync.Mutex

// type MatchPair struct {
// 	chair Chair
// 	ride  Ride
// }

// このAPIをインスタンス内から一定間隔で叩かせることで、椅子とライドをマッチングさせる
var isProcessing bool

func internalGetMatching(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	if isProcessing {
		mu.Unlock()
		w.WriteHeader(http.StatusForbidden)
		return
	}
	isProcessing = true
	mu.Unlock()

	defer func() {
		mu.Lock()
		isProcessing = false
		mu.Unlock()
	}()

	ctx := r.Context()
	chairs := freeChairsCache.List()
	if len(chairs) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	rides := []Ride{}
	if err := db.SelectContext(ctx, &rides, "SELECT * FROM rides WHERE chair_id IS NULL ORDER BY created_at LIMIT ?", 2*len(chairs)); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if len(rides) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	n := len(rides) + len(chairs) + 2
	// 最小費用流
	mcf := mcf.NewMinCostFlow(n)

	// source -> chair
	for i, _ := range chairs {
		mcf.AddEdge(0, i+1, 1, 0)
	}

	// chair -> ride
	for i, c := range chairs {
		for j, r := range rides {
			cLoc := getLatestChairLocation(c.ID)
			distance := calculateDistance(cLoc.Latitude, cLoc.Longitude, r.PickupLatitude, r.PickupLongitude)
			speed := 1
			if s, ok := chairSpeedbyName.Load(c.Model); ok {
				speed = s.(int)
			}
			time := distance / speed
			mcf.AddEdge(i+1, len(chairs)+j+1, 1, time)
		}
	}

	// ride -> sink
	for j, _ := range rides {
		mcf.AddEdge(len(chairs)+j+1, n-1, 1, 0)
	}
	// calc min path
	mcf.FlowL(0, n-1, mcf.Min(len(chairs), len(rides)))
	// match
	// matchPair := []MatchPair{}
	edges := mcf.Edges()
	isInit := true
	var chairIDsComma, rideIDsComma string
	for _, e := range edges {
		// 流量のあるEdgeだけを見る(source, sinkは除く)
		if e.Flow() == 0 || e.From() == 0 || e.To() == n-1 {
			continue
		}
		chair := chairs[e.From()-1]
		ride := rides[e.To()-len(chairs)-1]
		if isInit {
			isInit = false
		} else {
			chairIDsComma += ","
			rideIDsComma += ","
		}
		chairIDsComma += fmt.Sprintf("'%s'", chair.ID)
		rideIDsComma += fmt.Sprintf("'%s'", ride.ID)
		// matchPair = append(matchPair, MatchPair{chair, ride})
	}
	// var chairIDsComma, rideIDsComma string
	// for i, mp := range matchPair {
	// 	if i > 0 {
	// 		chairIDsComma += ","
	// 		rideIDsComma += ","
	// 	}
	// 	chairIDsComma += fmt.Sprintf("'%s'", mp.chairID)
	// 	rideIDsComma += fmt.Sprintf("'%s'", mp.rideID)
	// }
	// if _, err := db.ExecContext(ctx, fmt.Sprintf("UPDATE chairs SET is_completed = 0 WHERE id IN (%s)", chairIDsComma)); err != nil {
	// 	writeError(w, http.StatusInternalServerError, err)
	// 	return
	// }
	if _, err := db.ExecContext(ctx, fmt.Sprintf("UPDATE rides SET chair_id = ELT(FIELD(id, %s), %s) WHERE id IN (%s)", rideIDsComma, chairIDsComma, rideIDsComma)); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for _, e := range edges {
		// 流量のあるEdgeだけを見る(source, sinkは除く)
		if e.Flow() == 0 || e.From() == 0 || e.To() == n-1 {
			continue
		}
		chairID := chairs[e.From()-1].ID
		ride := rides[e.To()-len(chairs)-1]
		freeChairsCache.Remove(chairID)
		ride.ChairID = sql.NullString{String: chairID, Valid: true}
		latestRideCache.Store(chairID, ride)
		chairChan, ok := chairNotifChan.Load(chairID)
		if !ok {
			chairNotifChan.Store(chairID, make(chan Notif, 5))
			chairChan, _ = chairNotifChan.Load(chairID)
		}
		chairChan.(chan Notif) <- Notif{
			Ride: &ride,
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
