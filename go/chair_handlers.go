package main

import (
	"context"
	"database/sql"
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
	ctx := r.Context()
	req := &chairPostChairsRequest{}
	if err := bindJSON(r, req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.Model == "" || req.ChairRegisterToken == "" {
		writeError(w, http.StatusBadRequest, errors.New("some of required fields(name, model, chair_register_token) are empty"))
		return
	}

	owner := &Owner{}
	if err := db.GetContext(ctx, owner, "SELECT * FROM owners WHERE chair_register_token = ?", req.ChairRegisterToken); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, errors.New("invalid chair_register_token"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	chairID := ulid.Make().String()
	accessToken := secureRandomStr(32)

	_, err := db.ExecContext(
		ctx,
		"INSERT INTO chairs (id, owner_id, name, model, is_active, access_token) VALUES (?, ?, ?, ?, ?, ?)",
		chairID, owner.ID, req.Name, req.Model, false, accessToken,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

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

	_, err := db.ExecContext(ctx, "UPDATE chairs SET is_active = ? WHERE id = ?", req.IsActive, chair.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
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
	lazyDo := func() {}
	lazyDo2 := func() {}

	tx, err := db.Beginx()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer tx.Rollback()

	chairLocationID := ulid.Make().String()
	// if _, err := tx.ExecContext(
	// 	ctx,
	// 	`INSERT INTO chair_locations (id, chair_id, latitude, longitude) VALUES (?, ?, ?, ?)`,
	// 	chairLocationID, chair.ID, req.Latitude, req.Longitude,
	// );
	now := time.Now()
	lazyDo2, err = createChairLoc(ctx, tx, chairLocationID, chair.ID, req.Latitude, req.Longitude, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	location := &ChairLocation{
		ID:        chairLocationID,
		ChairID:   chair.ID,
		Latitude:  req.Latitude,
		Longitude: req.Longitude,
		CreatedAt: now,
	}
	// if err := tx.GetContext(ctx, location, `SELECT * FROM chair_locations WHERE id = ?`, chairLocationID); err != nil {
	// 	writeError(w, http.StatusInternalServerError, err)
	// 	return
	// }

	ride := &Ride{}
	// if err := tx.GetContext(ctx, ride, `SELECT * FROM rides WHERE chair_id = ? ORDER BY updated_at DESC LIMIT 1`, chair.ID); err != nil {
	// 	if !errors.Is(err, sql.ErrNoRows) {
	// 		writeError(w, http.StatusInternalServerError, err)
	// 		return
	// 	}
	// }
	rideIns, err := getLatestRideByChairID(ctx, tx, chair.ID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	} else {
		ride = &rideIns
		status, err := getLatestRideStatus(ctx, tx, ride.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		if status != "COMPLETED" && status != "CANCELED" {
			if req.Latitude == ride.PickupLatitude && req.Longitude == ride.PickupLongitude && status == "ENROUTE" {
				// if _, err := tx.ExecContext(ctx, "INSERT INTO ride_statuses (id, ride_id, status) VALUES (?, ?, ?)", ulid.Make().String(), ride.ID, "PICKUP"); err != nil {
				// 	writeError(w, http.StatusInternalServerError, err)
				// 	return
				// }
				lazyDo, err = createRideStatus(ctx, tx, ride, "PICKUP")
				if err != nil {
					writeError(w, http.StatusInternalServerError, err)
					return
				}
			}

			if req.Latitude == ride.DestinationLatitude && req.Longitude == ride.DestinationLongitude && status == "CARRYING" {
				// if _, err := tx.ExecContext(ctx, "INSERT INTO ride_statuses (id, ride_id, status) VALUES (?, ?, ?)", ulid.Make().String(), ride.ID, "ARRIVED"); err != nil {
				// 	writeError(w, http.StatusInternalServerError, err)
				// 	return
				// }
				lazyDo, err = createRideStatus(ctx, tx, ride, "ARRIVED")
				if err != nil {
					writeError(w, http.StatusInternalServerError, err)
					return
				}
			}
		}
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	lazyDo()
	lazyDo2()

	writeJSON(w, http.StatusOK, &chairPostCoordinateResponse{
		RecordedAt: location.CreatedAt.UnixMilli(),
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

func chairGetNotification(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chair := ctx.Value("chair").(*Chair)
	fmt.Printf("[DEBUG3] chairGetNotification 00: %s\n", chair.ID)
	// response, err := getChairNotification(ctx, chair)
	// if err != nil {
	// 	writeError(w, http.StatusInternalServerError, err)
	// 	return
	// }
	// writeJSON(w, http.StatusOK, response)

	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	clientGone := ctx.Done()
	rc := http.NewResponseController(w)

	// t := time.NewTicker(time.Second)
	// defer t.Stop()
	if _, ok := ChairNotifChan[chair.ID]; !ok {
		ChairNotifChan[chair.ID] = make(chan Notif, 5)
	}
	for {
		fmt.Printf("[DEBUG3] chairGetNotification loop\n")
		select {
		case <-clientGone:
			fmt.Println("Client disconnected")
			return
		// case <-t.C:
		case notif := <-ChairNotifChan[chair.ID]:
			fmt.Printf("[DEBUG3] chairGetNotification 01\n")
			response, err := getChairNotification(ctx, chair, notif.Ride)
			if err != nil {
				fmt.Printf("[DEBUG3] chairGetNotification err : %v\n", err)
				return
			}
			resV, err := json.Marshal(response.Data)
			if err != nil {
				return
			}
			fmt.Printf("data: %s\n", string(resV))
			_, err = fmt.Fprintf(w, "data: %s\n\n", string(resV))
			if err != nil {
				return
			}
			fmt.Printf("[DEBUG3] chairGetNotification 02\n")
			err = rc.Flush()
			if err != nil {
				return
			}
		}
	}
}

func getChairNotification(ctx context.Context, chair *Chair, ride *Ride) (*chairGetNotificationResponse, error) {
	tx, err := db.Beginx()
	if err != nil {
		// writeError(w, http.StatusInternalServerError, err)
		return nil, err
	}
	defer tx.Rollback()
	// ride := &Ride{}
	yetSentRideStatus := RideStatus{}
	status := ""

	// if err := tx.GetContext(ctx, ride, `SELECT * FROM rides WHERE chair_id = ? ORDER BY updated_at DESC LIMIT 1`, chair.ID); err != nil {
	// 	if errors.Is(err, sql.ErrNoRows) {
	// 		// writeJSON(w, http.StatusOK, &chairGetNotificationResponse{
	// 		// 	RetryAfterMs: 250,
	// 		// })
	// 		return &chairGetNotificationResponse{
	// 			RetryAfterMs: 250,
	// 		}, nil
	// 	}
	// 	// writeError(w, http.StatusInternalServerError, err)
	// 	return nil, err
	// }

	// rideIns, err := getLatestRideByChairID(ctx, tx, chair.ID)
	// if err != nil {
	// 	if errors.Is(err, sql.ErrNoRows) {
	// 		writeJSON(w, http.StatusOK, &chairGetNotificationResponse{
	// 			RetryAfterMs: 250,
	// 		})
	// 		return nil, err
	// 	}
	// writeError(w, http.StatusInternalServerError, err)
	// 	return nil, err
	// }
	// ride = &rideIns
	if err := tx.GetContext(ctx, &yetSentRideStatus, `SELECT * FROM ride_statuses WHERE ride_id = ? AND chair_sent_at IS NULL ORDER BY created_at ASC LIMIT 1`, ride.ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			status, err = getLatestRideStatus(ctx, tx, ride.ID)
			if err != nil {
				// writeError(w, http.StatusInternalServerError, err)
				return nil, err
			}
		} else {
			// writeError(w, http.StatusInternalServerError, err)
			return nil, err
		}
	} else {
		status = yetSentRideStatus.Status
	}

	user := &User{}
	err = tx.GetContext(ctx, user, "SELECT * FROM users WHERE id = ? FOR SHARE", ride.UserID)
	if err != nil {
		// writeError(w, http.StatusInternalServerError, err)
		return nil, err
	}

	if yetSentRideStatus.ID != "" {
		_, err := tx.ExecContext(ctx, `UPDATE ride_statuses SET chair_sent_at = CURRENT_TIMESTAMP(6) WHERE id = ?`, yetSentRideStatus.ID)
		if err != nil {
			// writeError(w, http.StatusInternalServerError, err)
			return nil, err
		}
		if yetSentRideStatus.Status == "COMPLETED" {
			if _, err = db.ExecContext(ctx, "UPDATE chairs SET is_completed = 1 WHERE id = ?", ride.ChairID.String); err != nil {
				// writeError(w, http.StatusInternalServerError, err)
				return nil, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		// writeError(w, http.StatusInternalServerError, err)
		return nil, err
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
			Status: status,
		},
		RetryAfterMs: 250,
	}, nil
}

type postChairRidesRideIDStatusRequest struct {
	Status string `json:"status"`
}

func chairPostRideStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rideID := r.PathValue("ride_id")

	chair := ctx.Value("chair").(*Chair)
	lazyDo := func() {}

	req := &postChairRidesRideIDStatusRequest{}
	if err := bindJSON(r, req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	tx, err := db.Beginx()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer tx.Rollback()

	ride := &Ride{}
	if err := tx.GetContext(ctx, ride, "SELECT * FROM rides WHERE id = ? FOR UPDATE", rideID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, errors.New("ride not found"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	if ride.ChairID.String != chair.ID {
		writeError(w, http.StatusBadRequest, errors.New("not assigned to this ride"))
		return
	}

	switch req.Status {
	// Acknowledge the ride
	case "ENROUTE":
		// if _, err := tx.ExecContext(ctx, "INSERT INTO ride_statuses (id, ride_id, status) VALUES (?, ?, ?)", ulid.Make().String(), ride.ID, "ENROUTE"); err != nil {
		// 	writeError(w, http.StatusInternalServerError, err)
		// 	return
		// }
		lazyDo, err = createRideStatus(ctx, tx, ride, "ENROUTE")
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	// After Picking up user
	case "CARRYING":
		status, err := getLatestRideStatus(ctx, tx, ride.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if status != "PICKUP" {
			writeError(w, http.StatusBadRequest, errors.New("chair has not arrived yet"))
			return
		}
		// if _, err := tx.ExecContext(ctx, "INSERT INTO ride_statuses (id, ride_id, status) VALUES (?, ?, ?)", ulid.Make().String(), ride.ID, "CARRYING"); err != nil {
		// 	writeError(w, http.StatusInternalServerError, err)
		// 	return
		// }
		lazyDo, err = createRideStatus(ctx, tx, ride, "CARRYING")
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	default:
		writeError(w, http.StatusBadRequest, errors.New("invalid status"))
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	lazyDo()

	w.WriteHeader(http.StatusNoContent)
}
