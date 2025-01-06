package main

import (
	"errors"
	"net/http"
	"time"

	"github.com/bytedance/sonic"
	"github.com/oklog/ulid/v2"
)

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

	owner, ok := getOwnerChairRegisterToken(req.ChairRegisterToken)
	if !ok {
		writeError(w, http.StatusUnauthorized, errors.New("invalid chair_register_token"))
		return
	}

	chairID := ulid.Make().String()
	accessToken := secureRandomStr(32)
	now := time.Now()
	chair := &Chair{
		ID:          chairID,
		OwnerID:     owner.ID,
		Name:        req.Name,
		Model:       req.Model,
		IsActive:    false,
		AccessToken: accessToken,
		CreatedAt:   now,
		UpdatedAt:   now,
		Speed:       getChairSpeedbyName(req.Model),
	}
	createChair(chairID, chair)
	createChairAccessToken(accessToken, chair)
	createChairsOwnerID(owner.ID, chair)

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

func chairPostActivity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chair := ctx.Value("chair").(*Chair)

	req := &postChairActivityRequest{}
	if err := bindJSON(r, req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if req.IsActive {
		freeChairs.Add(chair)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	freeChairs.Remove(chair.ID)
	w.WriteHeader(http.StatusNoContent)
}

func chairPostCoordinate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	req := &Coordinate{}
	chairPostCoordinateBindJSON(r, req)
	now := time.Now()
	chairPostCoordinateWriteJSON(w, now)
	chair := ctx.Value("chair").(*Chair)

	ride := &Ride{}
	ride, ok := getLatestRide(chair.ID)
	if ok {
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

	chairLocation := &ChairLocation{
		ID:        "dummy",
		ChairID:   chair.ID,
		Latitude:  req.Latitude,
		Longitude: req.Longitude,
		CreatedAt: now,
	}
	before, ok := getLatestChairLocation(chair.ID)
	createChairLocation(chair.ID, chairLocation)
	if ok {
		distance := calculateDistance(before.Latitude, before.Longitude, req.Latitude, req.Longitude)
		createChairTotalDistance(chair.ID, distance, now)
	}
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

	chairChan := getChairChan(chair.ID)
	for {
		select {
		case <-clientGone:
			return
		case notif := <-chairChan:
			response, err := getChairNotification(notif.Ride, notif.RideStatus)
			if err != nil {
				return
			}
			resV, err := sonic.Marshal(response.Data)
			if err != nil {
				return
			}
			if _, err := w.Write([]byte("data: ")); err != nil {
				return
			}
			if _, err := w.Write(resV); err != nil {
				return
			}
			if _, err := w.Write([]byte("\n\n")); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}
			if notif.RideStatus == "COMPLETED" {
				go func() {
					// evaluationの完了待ち
					time.Sleep(50 * time.Millisecond)
					freeChairs.Add(chair)
					deleteLatestRide(chair.ID)
				}()
			}
		}
	}
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

	ride, ok := getRide(rideID)
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
		processRideStatus(ride, targetStatus)
	}

	w.WriteHeader(http.StatusNoContent)
}
