package main

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"sync"
	"time"

	pb "github.com/isucon/isucon14/webapp/go/grpc"

	mcf "github.com/isucon/isucon14/webapp/go/mincostflow"
)

var mu sync.Mutex

func startMatchingLoop() {
	ticker := time.NewTicker(75 * time.Millisecond)
	for range ticker.C {
		mu.Lock()
		if ok := internalGetMatchingOutsource(); ok {
			mu.Unlock()
			break
		}
		mu.Unlock()
	}
	ticker2 := time.NewTicker(30 * time.Millisecond)
	for range ticker2.C {
		mu.Lock()
		internalGetMatchingGreedy()
		mu.Unlock()
	}
}

func internalGetMatchingOutsource() (isNoAgeLimit bool) {
	freeChairs.Lock()
	chairs := freeChairs.List()
	freeChairs.Unlock()
	if len(chairs) < 5 {
		return
	}
	rides := waitingRides.List()
	if len(rides) == 0 {
		return
	}
	fmt.Printf("[DEBUG1] chairs, rides: %d, %d\n", len(chairs), len(rides))
	slices.SortFunc(rides, func(a, b *Ride) int {
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		return 0
	})
	// if rides[0].CreatedAt.After(benchStartedAt.Add(35 * time.Second)) {
	if len(chairs) > 100 {
		isNoAgeLimit = true
		return
	}
	min := 5 * len(chairs)
	if len(rides) < min {
		min = len(rides)
	}
	rides = rides[:min]
	matchableChair := []*pb.MatchableChair{}
	matchableRide := []*pb.MatchableRide{}

	for _, c := range chairs {
		coord, ok := getLatestChairLocation(c.ID)
		if !ok {
			continue
		}
		matchableChair = append(matchableChair, &pb.MatchableChair{
			Id:    c.ID,
			Model: c.Model,
			Coordinate: &pb.Coordinate{
				Latitude:  int32(coord.Latitude),
				Longitude: int32(coord.Longitude),
			},
			CreatedAt: c.CreatedAt.Unix(),
		})
	}
	for _, r := range rides {
		matchableRide = append(matchableRide, &pb.MatchableRide{
			Id: r.ID,
			Coordinate: &pb.Coordinate{
				Latitude:  int32(r.PickupLatitude),
				Longitude: int32(r.PickupLongitude),
			},
		})
	}

	pair, err := client.MinCostFlow(context.Background(),
		&pb.MinCostFlowRequest{
			Chairs: matchableChair,
			Rides:  matchableRide,
		},
	)
	if err != nil {
		fmt.Printf("%v\n", err)
		return
	}
	// match
	for _, p := range pair.GetRideChairs() {
		chairID := p.ChairID
		ride, _ := getRide(p.RideID)

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
	return
}

type MatchableChair struct {
	ID        string
	Speed     int
	Latitude  int
	Longitude int
}

func internalGetMatchingSelf() {
	freeChairs.Lock()
	chairs := freeChairs.List()
	freeChairs.Unlock()
	if len(chairs) < 5 {
		return
	}
	rides := waitingRides.List()
	if len(rides) == 0 {
		return
	}

	fmt.Printf("[DEBUG1] chairs, rides: %d, %d\n", len(chairs), len(rides))
	slices.SortFunc(rides, func(a, b *Ride) int {
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		return 0
	})
	matchableChairs := []*MatchableChair{}
	for _, c := range chairs {
		coord, ok := getLatestChairLocation(c.ID)
		if !ok {
			continue
		}
		matchableChairs = append(matchableChairs, &MatchableChair{
			ID:        c.ID,
			Speed:     c.Speed,
			Latitude:  coord.Latitude,
			Longitude: coord.Longitude,
		})
	}
	chairsCount := len(matchableChairs)
	min := 5 * chairsCount
	if len(rides) < min {
		min = len(rides)
	}
	rides = rides[:min]

	ridesCount := min
	n := ridesCount + chairsCount + 2
	// 最小費用流
	mcf := mcf.NewMinCostFlow(n)

	// source -> chair
	for i := range chairsCount {
		mcf.AddEdge(0, i+1, 1, 0)
	}

	// chair -> ride
	for i, c := range matchableChairs {
		for j, r := range rides {
			distance := calculateDistance(c.Latitude, c.Longitude, r.PickupLatitude, r.PickupLongitude)
			time := distance / c.Speed
			mcf.AddEdge(i+1, chairsCount+j+1, 1, time)
		}
	}

	// ride -> sink
	for j := range ridesCount {
		mcf.AddEdge(chairsCount+j+1, n-1, 1, 0)
	}

	// calc min path
	mcf.FlowL(0, n-1, mcf.Min(chairsCount, ridesCount))

	// match
	edges := mcf.Edges()
	for _, e := range edges {
		// 流量のあるEdgeだけを見る(source, sinkは除く)
		if e.Flow() == 0 || e.From() == 0 || e.To() == n-1 {
			continue
		}
		chairID := chairs[e.From()-1].ID
		ride := rides[e.To()-chairsCount-1]

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
}

func internalGetMatchingGreedy() {
	freeChairs.Lock()
	chairs := freeChairs.List()
	freeChairs.Unlock()
	if len(chairs) < 5 {
		return
	}
	rides := waitingRides.List()
	if len(rides) == 0 {
		return
	}

	fmt.Printf("[DEBUG2] chairs, rides: %d, %d\n", len(chairs), len(rides))
	matchableChairs := []*MatchableChair{}
	for _, c := range chairs {
		coord, ok := getLatestChairLocation(c.ID)
		if !ok {
			continue
		}
		matchableChairs = append(matchableChairs, &MatchableChair{
			ID:        c.ID,
			Speed:     c.Speed,
			Latitude:  coord.Latitude,
			Longitude: coord.Longitude,
		})
	}
	slices.SortFunc(matchableChairs, func(a, b *MatchableChair) int {
		if a.Speed > b.Speed {
			return -1
		}
		return 0
	})
	matched := map[int]bool{}
	for _, c := range matchableChairs {
		minDistance := 1000000000
		matchRideIdx := -1
		for j, r := range rides {
			if matched[j] {
				continue
			}
			distance := calculateDistance(c.Latitude, c.Longitude, r.PickupLatitude, r.PickupLongitude)
			if distance < minDistance {
				minDistance = distance
				matchRideIdx = j
			}
		}
		matched[matchRideIdx] = true
		chairID := c.ID
		ride := rides[matchRideIdx]
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
}
