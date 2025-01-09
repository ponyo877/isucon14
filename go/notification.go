package main

import (
	"net/http"
	"time"

	"github.com/bytedance/sonic"
)

func appGetNotification(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := ctx.Value("user").(*User)

	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	clientGone := ctx.Done()
	rc := http.NewResponseController(w)
	appChan := getAppChan(user.ID)
	for {
		select {
		case <-clientGone:
			return
		case notif := <-appChan:
			response, err := getAppNotification(user, notif.Ride, notif.RideStatus)
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
				deleteLatestRide(notif.Ride.ChairID.String)
			}
		}
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
