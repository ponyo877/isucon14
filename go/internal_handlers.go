package main

import (
	"fmt"
	"net/http"
	"sync"

	mcf "github.com/isucon/isucon14/webapp/go/mincostflow"
)

var mu sync.Mutex

type MatchPair struct {
	chairID string
	rideID  string
}

// このAPIをインスタンス内から一定間隔で叩かせることで、椅子とライドをマッチングさせる
func internalGetMatching(w http.ResponseWriter, r *http.Request) {
	// 複数スレッドで同時実行されないように排他制御
	if !mu.TryLock() {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	defer mu.Unlock()
	ctx := r.Context()
	var chairs []Chair
	if err := db.SelectContext(ctx, &chairs, `
		select *
		from chairs
		where is_completed = 1
		and   is_active = 1`); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
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
	matchPair := []MatchPair{}
	for _, e := range mcf.Edges() {
		// 流量のあるEdgeだけを見る(source, sinkは除く)
		if e.Flow() == 0 || e.From() == 0 || e.To() == n-1 {
			continue
		}
		chairID := chairs[e.From()-1].ID
		rideID := rides[e.To()-len(chairs)-1].ID
		matchPair = append(matchPair, MatchPair{chairID, rideID})
	}
	var chairIDsComma, rideIDsComma string
	for i, mp := range matchPair {
		if i > 0 {
			chairIDsComma += ","
			rideIDsComma += ","
		}
		chairIDsComma += fmt.Sprintf("'%s'", mp.chairID)
		rideIDsComma += fmt.Sprintf("'%s'", mp.rideID)
		latestRideCache.Delete(mp.chairID)
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf("UPDATE chairs SET is_completed = 0 WHERE id IN (%s)", chairIDsComma)); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf("UPDATE rides SET chair_id = ELT(FIELD(id, %s), %s) WHERE id IN (%s)", rideIDsComma, chairIDsComma, rideIDsComma)); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for _, e := range mcf.Edges() {
		// 流量のあるEdgeだけを見る(source, sinkは除く)
		if e.Flow() == 0 || e.From() == 0 || e.To() == n-1 {
			continue
		}
		chairID := chairs[e.From()-1].ID
		ride := rides[e.To()-len(chairs)-1]
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
