package main

import (
	"bufio"
	"fmt"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v2"
	"github.com/oklog/ulid/v2"
	"github.com/valyala/fasthttp"
)

func chairPostChairsFiber(c *fiber.Ctx) error {
	req := &chairPostChairsRequest{}
	if err := c.BodyParser(req); err != nil {
		return c.SendStatus(fiber.StatusBadRequest)
	}
	if req.Name == "" || req.Model == "" || req.ChairRegisterToken == "" {
		return c.SendStatus(fiber.StatusBadRequest)
	}

	owner, ok := getOwnerChairRegisterToken(req.ChairRegisterToken)
	if !ok {
		return c.SendStatus(fiber.StatusUnauthorized)
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
		// Path:  "/",
		Name:  "chair_session",
		Value: accessToken,
	})

	return c.Status(fiber.StatusCreated).JSON(&chairPostChairsResponse{
		ID:      chairID,
		OwnerID: owner.ID,
	})
}

func chairPostActivityFiber(c *fiber.Ctx) error {
	chair := c.Locals("chair").(*Chair)

	req := &postChairActivityRequest{}
	if err := c.BodyParser(req); err != nil {
		return c.SendStatus(fiber.StatusBadRequest)
	}
	if req.IsActive {
		freeChairs.Add(chair)
		return c.SendStatus(fiber.StatusNoContent)
	}
	freeChairs.Remove(chair.ID)
	return c.SendStatus(fiber.StatusNoContent)
}

func chairPostCoordinateBindJSONFiber(body []byte, req *Coordinate) {
	len := len(body)
	pos := posComma(body)
	req.Latitude = byteToInt(body[12:pos])
	req.Longitude = byteToInt(body[pos+13 : len-1])
}

func chairPostCoordinateFiber(c *fiber.Ctx) error {
	req := &Coordinate{}
	// if err := c.BodyParser(req); err != nil {
	// 	return c.SendStatus(fiber.StatusBadRequest)
	// }
	chairPostCoordinateBindJSONFiber(c.Body(), req)

	chair := c.Locals("chair").(*Chair)

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
	id := ulid.Make().String()
	now := time.Now()
	chairLocation := &ChairLocation{
		ID:        id,
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
	return c.JSON(&chairPostCoordinateResponse{
		RecordedAt: now.UnixMilli(),
	})
	// raw := []byte(`{"recorded_at":` + strconv.FormatInt(now.UnixMilli(), 10) + "}")
	// c.fasthttp.Response.SetBodyRaw(raw)
	// return nil
}

// SSE
func chairGetNotificationFiber(c *fiber.Ctx) error {
	chair := c.Locals("chair").(*Chair)

	c.Set("X-Accel-Buffering", "no")
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	clientGone := c.Context().Done()
	chairChan := getChairChan(chair.ID)
	c.Status(fiber.StatusOK).Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
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
				// if _, err := w.Write([]byte("data: ")); err != nil {
				// 	return
				// }
				// if _, err := w.Write(resV); err != nil {
				// 	return
				// }
				// if _, err := w.Write([]byte("\n\n")); err != nil {
				// 	return
				// }
				if _, err := fmt.Fprintf(w, "data: %s\n\n", string(resV)); err != nil {
					fmt.Printf("[DEBUG] Fprintf\n")
					return
				}
				// fmt.Fprintf(w, "data: Message: %s\n\n", msg)
				if err := w.Flush(); err != nil {
					return
				}
				if notif.RideStatus == "COMPLETED" {
					go func() {
						// evaluationの完了待ち
						// time.Sleep(50 * time.Millisecond)
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
	rideID := c.Params("ride_id")
	chair := c.Locals("chair").(*Chair)

	req := &postChairRidesRideIDStatusRequest{}
	if err := c.BodyParser(req); err != nil {
		return c.SendStatus(fiber.StatusBadRequest)
	}

	ride, ok := getRide(rideID)
	if !ok {
		return c.SendStatus(fiber.StatusNotFound)
	}

	if ride.ChairID.String != chair.ID {
		return c.SendStatus(fiber.StatusBadRequest)
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
			return c.SendStatus(fiber.StatusBadRequest)
		}
		targetStatus = "CARRYING"
	default:
		return c.SendStatus(fiber.StatusBadRequest)
	}

	if targetStatus != "" {
		processRideStatus(ride, targetStatus)
	}

	return c.SendStatus(fiber.StatusNoContent)
}
