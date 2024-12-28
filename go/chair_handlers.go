package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/oklog/ulid/v2"
)

type chairPostChairsRequest struct {
	Name               string `json:"name"`
	Model              string `json:"model"`
	ChairRegisterToken string `json:"chair_register_token"`
}

type chairPostChairsResponse struct {
	ID      string `json:"id"`
	OwnerID string `json:"owner_id"`
}

func chairPostChairs(w http.ResponseWriter, r *http.Request) {
	req := &chairPostChairsRequest{}
	if err := bindJSON(r, req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.Model == "" || req.ChairRegisterToken == "" {
		writeError(w, http.StatusBadRequest, errors.New("some of required fields(name, model, chair_register_token) are empty"))
		return
	}

	owner, ok := getOwnerChairRegisterTokenCache(req.ChairRegisterToken)
	if !ok {
		writeError(w, http.StatusUnauthorized, errors.New("invalid chair_register_token"))
		return
	}

	chairID := ulid.Make().String()
	accessToken := secureRandomStr(32)
	now := time.Now()
	chair := Chair{
		ID:          chairID,
		OwnerID:     owner.ID,
		Name:        req.Name,
		Model:       req.Model,
		IsActive:    false,
		AccessToken: accessToken,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	createChairCache(chairID, chair)
	createChairAccessToken(accessToken, chair)
	createChairsOwnerIDCache(owner.ID, chair)

	http.SetCookie(w, &http.Cookie{
		Path:  "/",
		Name:  "chair_session",
		Value: accessToken,
	})

	writeJSON(w, http.StatusCreated, &chairPostChairsResponse{
		ID:      chairID,
		OwnerID: owner.ID,
	})
}

type postChairActivityRequest struct {
	IsActive bool `json:"is_active"`
}

func chairPostActivity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chair := ctx.Value("chair").(*Chair)

	req := &postChairActivityRequest{}
	if err := bindJSON(r, req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if req.IsActive {
		freeChairsCache.Add(*chair)
	} else {
		freeChairsCache.Remove(chair.ID)
	}

	w.WriteHeader(http.StatusNoContent)
}

type chairPostCoordinateResponse struct {
	RecordedAt int64 `json:"recorded_at"`
}

func chairPostCoordinate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	req := &Coordinate{}
	if err := bindJSON(r, req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	chair := ctx.Value("chair").(*Chair)

	ride := &Ride{}
	rideAny, ok := latestRideCache.Load(chair.ID)
	if ok {
		rideIns := rideAny.(Ride)
		ride = &rideIns
		status, _ := getLatestRideStatus(ride.ID)
		if status != "COMPLETED" && status != "CANCELED" {
			if req.Latitude == ride.PickupLatitude && req.Longitude == ride.PickupLongitude && status == "ENROUTE" {
				processRideStatus(ride, "PICKUP")
			}

			if req.Latitude == ride.DestinationLatitude && req.Longitude == ride.DestinationLongitude && status == "CARRYING" {
				processRideStatus(ride, "ARRIVED")
			}
		}
	}
	id := ulid.Make().String()
	now := time.Now()
	chairLocation := ChairLocation{
		ID:        id,
		ChairID:   chair.ID,
		Latitude:  req.Latitude,
		Longitude: req.Longitude,
		CreatedAt: now,
	}
	before, ok := getLatestChairLocationChacke(chair.ID)
	createChairLocation(chair.ID, chairLocation)
	if ok {
		distance := calculateDistance(before.Latitude, before.Longitude, req.Latitude, req.Longitude)
		createChairTotalDistanceCache(chair.ID, distance, now)
	}

	writeJSON(w, http.StatusOK, &chairPostCoordinateResponse{
		RecordedAt: now.UnixMilli(),
	})
}

type simpleUser struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type chairGetNotificationResponse struct {
	Data         *chairGetNotificationResponseData `json:"data"`
	RetryAfterMs int                               `json:"retry_after_ms"`
}

type chairGetNotificationResponseData struct {
	RideID                string     `json:"ride_id"`
	User                  simpleUser `json:"user"`
	PickupCoordinate      Coordinate `json:"pickup_coordinate"`
	DestinationCoordinate Coordinate `json:"destination_coordinate"`
	Status                string     `json:"status"`
}

// SSE
func chairGetNotification(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chair := ctx.Value("chair").(*Chair)

	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	clientGone := ctx.Done()
	rc := http.NewResponseController(w)

	chairChan, ok := chairNotifChan.Load(chair.ID)
	if !ok {
		chairNotifChan.Store(chair.ID, make(chan Notif, 5))
		chairChan, _ = chairNotifChan.Load(chair.ID)
	}
	for {
		select {
		case <-clientGone:
			return
		case notif := <-chairChan.(chan Notif):
			response, err := getChairNotification(notif.Ride)
			if err != nil {
				return
			}
			resV, err := json.Marshal(response.Data)
			if err != nil {
				return
			}
			_, err = fmt.Fprintf(w, "data: %s\n\n", string(resV))
			if err != nil {
				return
			}
			err = rc.Flush()
			if err != nil {
				return
			}
			if notif.RideStatus == "COMPLETED" {
				go func() {
					// evaluationの完了待ち
					time.Sleep(50 * time.Millisecond)
					freeChairsCache.Add(*chair)
					latestRideCache.Delete(chair.ID)
				}()
			}
		}
	}
}

func getChairNotification(ride *Ride) (*chairGetNotificationResponse, error) {
	rideStatus, _ := getLatestRideStatus(ride.ID)

	user, ok := getUserCache(ride.UserID)
	if !ok {
		return nil, errors.New("user not found")
	}

	return &chairGetNotificationResponse{
		Data: &chairGetNotificationResponseData{
			RideID: ride.ID,
			User: simpleUser{
				ID:   user.ID,
				Name: fmt.Sprintf("%s %s", user.Firstname, user.Lastname),
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

type postChairRidesRideIDStatusRequest struct {
	Status string `json:"status"`
}

func chairPostRideStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rideID := r.PathValue("ride_id")

	chair := ctx.Value("chair").(*Chair)

	req := &postChairRidesRideIDStatusRequest{}
	if err := bindJSON(r, req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	ride, ok := getRideCache(rideID)
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("ride not found"))
		return
	}

	if ride.ChairID.String != chair.ID {
		writeError(w, http.StatusBadRequest, errors.New("not assigned to this ride"))
		return
	}

	var targetStatus string
	switch req.Status {
	// Acknowledge the ride
	case "ENROUTE":
		targetStatus = "ENROUTE"
	// After Picking up user
	case "CARRYING":
		status, _ := getLatestRideStatus(ride.ID)
		if status != "PICKUP" {
			writeError(w, http.StatusBadRequest, errors.New("chair has not arrived yet"))
			return
		}
		targetStatus = "CARRYING"
	default:
		writeError(w, http.StatusBadRequest, errors.New("invalid status"))
	}

	if targetStatus != "" {
		processRideStatus(&ride, targetStatus)
	}

	w.WriteHeader(http.StatusNoContent)
}
