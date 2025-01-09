package main

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/oklog/ulid/v2"
)

func chairPostChairs(c *fiber.Ctx) error {
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

func chairPostActivity(c *fiber.Ctx) error {
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

func chairPostCoordinate(c *fiber.Ctx) error {
	ctx := c.Context()
	req := &Coordinate{}
	chairPostCoordinateBindJSON(c.Body(), req)
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

func chairPostRideStatus(c *fiber.Ctx) error {
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
