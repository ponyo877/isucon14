package main

import (
	"errors"
)

// マンハッタン距離を求める
func calculateDistance(aLatitude, aLongitude, bLatitude, bLongitude int) int {
	return abs(aLatitude-bLatitude) + abs(aLongitude-bLongitude)
}

func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

func getAppNotification(user *User, ride *Ride, rideStatus string) (*appGetNotificationResponse, error) {
	response := &appGetNotificationResponse{
		Data: &appGetNotificationResponseData{
			RideID: ride.ID,
			PickupCoordinate: Coordinate{
				Latitude:  ride.PickupLatitude,
				Longitude: ride.PickupLongitude,
			},
			DestinationCoordinate: Coordinate{
				Latitude:  ride.DestinationLatitude,
				Longitude: ride.DestinationLongitude,
			},
			Fare:      ride.Fare,
			Status:    rideStatus,
			CreatedAt: ride.CreatedAt.UnixMilli(),
			UpdateAt:  ride.UpdatedAt.UnixMilli(),
		},
	}

	if ride.ChairID.Valid {
		chair, _ := getChair(ride.ChairID.String)
		stats := getChairStats(chair.ID)
		response.Data.Chair = &appGetNotificationResponseChair{
			ID:    chair.ID,
			Name:  chair.Name,
			Model: chair.Model,
			Stats: stats,
		}
	}

	return response, nil
}

func getChairStats(chairID string) appGetNotificationResponseChairStats {
	stats := appGetNotificationResponseChairStats{}
	if statsCache, ok := getChairStatsCache(chairID); ok {
		stats.TotalRidesCount = statsCache.RideCount
		stats.TotalEvaluationAvg = statsCache.TotalEvaluation / float64(statsCache.RideCount)
	}
	return stats
}

func calculateFare(pickupLatitude, pickupLongitude, destLatitude, destLongitude int) int {
	meteredFare := farePerDistance * calculateDistance(pickupLatitude, pickupLongitude, destLatitude, destLongitude)
	return initialFare + meteredFare
}

func posComma(b []byte) int {
	if b[13] == 44 {
		return 13
	}
	if b[14] == 44 {
		return 14
	}
	if b[15] == 44 {
		return 15
	}
	return -1
}

func byteToInt(b []byte) int {
	sign := 1
	if b[0] == 45 {
		b = b[1:]
		sign = -1
	}
	n := 0
	for _, ch := range b {
		ch -= '0'
		n = n*10 + int(ch)
	}
	return sign * n
}

func chairPostCoordinateBindJSON(body []byte, req *Coordinate) {
	len := len(body)
	pos := posComma(body)
	req.Latitude = byteToInt(body[12:pos])
	req.Longitude = byteToInt(body[pos+13 : len-1])
}

func getChairNotification(ride *Ride, rideStatus string) (*chairGetNotificationResponse, error) {
	user, ok := getUser(ride.UserID)
	if !ok {
		return nil, errors.New("user not found")
	}

	return &chairGetNotificationResponse{
		Data: &chairGetNotificationResponseData{
			RideID: ride.ID,
			User: simpleUser{
				ID:   user.ID,
				Name: user.Firstname + " " + user.Lastname,
			},
			PickupCoordinate: Coordinate{
				Latitude:  ride.PickupLatitude,
				Longitude: ride.PickupLongitude,
			},
			DestinationCoordinate: Coordinate{
				Latitude:  ride.DestinationLatitude,
				Longitude: ride.DestinationLongitude,
			},
			Status: rideStatus,
		},
	}, nil
}

const (
	initialFare     = 500
	farePerDistance = 100
)

func sumSales(rides []Ride) int {
	sale := 0
	for _, ride := range rides {
		sale += calculateSale(ride)
	}
	return sale
}

func calculateSale(ride Ride) int {
	return calculateFare(ride.PickupLatitude, ride.PickupLongitude, ride.DestinationLatitude, ride.DestinationLongitude)
}
