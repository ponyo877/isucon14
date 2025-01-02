package main

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"sync"
	"time"

	pb "github.com/isucon/isucon14/webapp/go/grpc"
)

var mu sync.Mutex

func startMatchingLoop() {
	ticker := time.NewTicker(75 * time.Millisecond)
	for range ticker.C {
		mu.Lock()
		internalGetMatching()
		mu.Unlock()
	}
}

func internalGetMatching() {
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
	fmt.Printf("[DEBUG] chairs, rides: %d, %d\n", len(chairs), len(rides))
	slices.SortFunc(rides, func(a, b *Ride) int {
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		return 0
	})
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
		fmt.Printf("[DEBUG] %v\n", err)
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
}
