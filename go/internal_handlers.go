package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"slices"
	"sync"

	mcf "github.com/isucon/isucon14/webapp/go/mincostflow"
)

var mu sync.Mutex

// このAPIをインスタンス内から一定間隔で叩かせることで、椅子とライドをマッチングさせる
var isProcessing bool

func internalGetMatching(w http.ResponseWriter, r *http.Request) {
	freeChairs.Lock()
	chairs := freeChairs.List()
	freeChairs.Unlock()
	if len(chairs) < 5 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	rides := waitingRides.List()
	if len(rides) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	fmt.Printf("[DEBUG] chairs, rides: %d, %d\n", len(chairs), len(rides))
	slices.SortFunc(rides, func(a, b *Ride) int {
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		return 0
	})
	min := 2 * len(chairs)
	if len(rides) < min {
		min = len(rides)
	}
	rides = rides[:min]

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
			cLoc, _ := getLatestChairLocation(c.ID)
			distance := calculateDistance(cLoc.Latitude, cLoc.Longitude, r.PickupLatitude, r.PickupLongitude)
			speed := 1
			if s, ok := getChairSpeedbyName(c.Model); ok {
				speed = s
			}
			getChairSpeedbyName(c.Model)
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
		chairIDsComma += "'" + chair.ID + "'"
		rideIDsComma += "'" + ride.ID + "'"
	}
	for _, e := range edges {
		// 流量のあるEdgeだけを見る(source, sinkは除く)
		if e.Flow() == 0 || e.From() == 0 || e.To() == n-1 {
			continue
		}
		chairID := chairs[e.From()-1].ID
		ride := rides[e.To()-len(chairs)-1]

		ride.ChairID = sql.NullString{String: chairID, Valid: true}
		createLatestRide(chairID, ride)
		freeChairs.Remove(chairID)
		waitingRides.Remove(ride.ID)
		createRide(ride.ID, ride)
		createUserRideStatus(ride.UserID, false)
		notif := &Notif{
			Ride:       ride,
			RideStatus: "MATCHING",
		}
		publishChairChan(chairID, notif)
		publishAppChan(ride.UserID, notif)
	}
	w.WriteHeader(http.StatusNoContent)
}
