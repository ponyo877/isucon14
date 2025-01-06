package main

import (
	"bufio"
	"net/http"
	"strconv"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v2"
	"github.com/oklog/ulid/v2"
	"github.com/valyala/fasthttp"
)

func chairPostChairsFiber(c *fiber.Ctx) error {
	req := &chairPostChairsRequest{}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest)
	}
	if req.Name == "" || req.Model == "" || req.ChairRegisterToken == "" {
		return fiber.NewError(http.StatusBadRequest, "some of required fields(name, model, chair_register_token) are empty")
	}

	owner, ok := getOwnerChairRegisterToken(req.ChairRegisterToken)
	if !ok {
		return fiber.NewError(http.StatusUnauthorized, "invalid chair_register_token")
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

	c.Cookie(&fiber.Cookie{
		Path:  "/",
		Name:  "chair_session",
		Value: accessToken,
	})

	return c.Status(http.StatusCreated).JSON(&chairPostChairsResponse{
		ID:      chairID,
		OwnerID: owner.ID,
	})
}

func chairPostActivityFiber(c *fiber.Ctx) error {
	ctx := c.Context()
	chair := ctx.UserValue("chair").(*Chair)

	req := &postChairActivityRequest{}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest)
	}
	if req.IsActive {
		freeChairs.Add(chair)
		return c.SendStatus(http.StatusNoContent)
	}
	freeChairs.Remove(chair.ID)
	return c.SendStatus(http.StatusNoContent)
}

func chairPostCoordinateFiber(c *fiber.Ctx) error {
	ctx := c.Context()
	req := &Coordinate{}
	chairPostCoordinateBindJSONFiber(c.Body(), req)
	// if err := c.BodyParser(&req); err != nil {
	// 	return fiber.NewError(http.StatusBadRequest)
	// }
	now := time.Now()
	defer func() {
		chair := ctx.UserValue("chair").(*Chair)

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
	}()
	// return c.Status(http.StatusOK).JSON(&chairPostCoordinateResponse{
	// 	RecordedAt: now.UnixMilli(),
	// })
	c.Response().Header.SetContentType("application/json;charset=utf-8")
	c.Response().SetBodyRaw([]byte(`{"recorded_at":` + strconv.FormatInt(now.UnixMilli(), 10) + `}`))
	return c.SendStatus(http.StatusOK)
}

// SSE
func chairGetNotificationFiber(c *fiber.Ctx) error {
	ctx := c.Context()
	chair := ctx.UserValue("chair").(*Chair)

	c.Response().Header.Set("X-Accel-Buffering", "no")
	c.Response().Header.Set(fiber.HeaderContentType, "text/event-stream")
	c.Response().Header.Set(fiber.HeaderCacheControl, "no-cache")
	c.Response().Header.Set(fiber.HeaderConnection, "keep-alive")

	clientGone := ctx.Done()
	// rc := http.NewResponseController(w)

	chairChan := getChairChan(chair.ID)
	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
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
				if err := w.Flush(); err != nil {
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
	}))
	return nil
}

func chairPostRideStatusFiber(c *fiber.Ctx) error {
	ctx := c.Context()
	rideID := c.Params("ride_id")

	chair := ctx.UserValue("chair").(*Chair)

	req := &postChairRidesRideIDStatusRequest{}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest, err.Error())
	}

	ride, ok := getRide(rideID)
	if !ok {
		return fiber.NewError(http.StatusNotFound, "ride not found")
	}

	if ride.ChairID.String != chair.ID {
		return fiber.NewError(http.StatusBadRequest, "not assigned to this ride")
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
			return fiber.NewError(http.StatusBadRequest, "chair has not arrived yet")
		}
		targetStatus = "CARRYING"
	default:
		return fiber.NewError(http.StatusBadRequest, "invalid status")
	}

	if targetStatus != "" {
		processRideStatus(ride, targetStatus)
	}

	return c.SendStatus(http.StatusNoContent)
}
