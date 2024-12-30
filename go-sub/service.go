package main

import (
	"context"

	pb "github.com/ponyo877/isucon14/go-sub/grpc"
	mcf "github.com/ponyo877/isucon14/go-sub/mincostflow"
)

type SubServer struct {
	pb.UnimplementedSubServiceServer
}

func NewServer() *SubServer {
	return &SubServer{}
}

func (s *SubServer) AppNotification(ctx context.Context, in *pb.AppNotificationRequest) (*pb.AppNotificationResponse, error) {
	return nil, nil
}

func (s *SubServer) ChairNotification(ctx context.Context, in *pb.ChairNotificationRequest) (*pb.ChairNotificationResponse, error) {
	return nil, nil
}

func (s *SubServer) MinCostFlow(ctx context.Context, in *pb.MinCostFlowRequest) (*pb.MinCostFlowResponse, error) {
	chairs := in.Chairs
	rides := in.Rides
	ridesCount := len(rides)
	chairsCount := len(chairs)
	n := ridesCount + chairsCount + 2
	// 最小費用流
	mcf := mcf.NewMinCostFlow(n)

	// source -> chair
	for i := range chairsCount {
		mcf.AddEdge(0, i+1, 1, 0)
	}

	// chair -> ride
	for i, c := range chairs {
		chairCoord := c.GetCoordinate()
		for j, r := range rides {
			rideCoord := r.GetCoordinate()
			distance := calculateDistance(chairCoord.Latitude, chairCoord.Longitude, rideCoord.Latitude, rideCoord.Longitude)
			speed := getChairSpeedbyName(c.Model)
			time := distance / speed
			mcf.AddEdge(i+1, len(chairs)+j+1, 1, time)
		}
	}

	// ride -> sink
	for j := range ridesCount {
		mcf.AddEdge(len(chairs)+j+1, n-1, 1, 0)
	}

	// calc min path
	mcf.FlowL(0, n-1, mcf.Min(len(chairs), len(rides)))

	// match
	edges := mcf.Edges()
	rideChairs := []*pb.RideChair{}
	for _, e := range edges {
		// 流量のあるEdgeだけを見る(source, sinkは除く)
		if e.Flow() == 0 || e.From() == 0 || e.To() == n-1 {
			continue
		}
		chair := chairs[e.From()-1]
		ride := rides[e.To()-len(chairs)-1]
		rideChairs = append(rideChairs, &pb.RideChair{
			ChairID: chair.GetId(),
			RideID:  ride.GetId(),
		})
	}
	return &pb.MinCostFlowResponse{
		RideChairs: rideChairs,
	}, nil
}

func (s *SubServer) StoreUserToken(ctx context.Context, in *pb.StoreUserTokenRequest) (*pb.StoreUserTokenResponse, error) {
	return nil, nil
}

func (s *SubServer) StoreChairToken(ctx context.Context, in *pb.StoreChairTokenRequest) (*pb.StoreChairTokenResponse, error) {
	return nil, nil
}

func calculateDistance(aLatitude, aLongitude, bLatitude, bLongitude int32) int {
	return abs(aLatitude-bLatitude) + abs(aLongitude-bLongitude)
}

func abs(a int32) int {
	if a < 0 {
		return int(-a)
	}
	return int(a)
}
