package main

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/oklog/ulid/v2"
)

func appPostUsers(c *fiber.Ctx) error {
	req := &appPostUsersRequest{}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest, err.Error())
	}
	if req.Username == "" || req.FirstName == "" || req.LastName == "" || req.DateOfBirth == "" {
		return fiber.NewError(http.StatusBadRequest, "required fields(username, firstname, lastname, date_of_birth) are empty")
	}

	userID := ulid.Make().String()
	accessToken := secureRandomStr(32)
	invitationCode := secureRandomStr(15)

	now := time.Now()
	user := &User{
		ID:             userID,
		Username:       req.Username,
		Firstname:      req.FirstName,
		Lastname:       req.LastName,
		DateOfBirth:    req.DateOfBirth,
		AccessToken:    accessToken,
		InvitationCode: invitationCode,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	createUser(userID, user)
	createUserInv(invitationCode, user)
	createUserRideStatus(userID, true)

	// 初回登録キャンペーンのクーポンを付与
	addUnusedCoupon(userID, 3000)

	// 招待コードを使った登録
	if req.InvitationCode != nil && *req.InvitationCode != "" {
		// 招待する側の招待数をチェック
		count, _ := getInvCouponCount(*req.InvitationCode)
		if count >= 3 {
			return fiber.NewError(http.StatusInternalServerError, "この招待コードは使用できません。")
		}

		// ユーザーチェック
		inviter, ok := getUserInv(*req.InvitationCode)
		if !ok {
			return fiber.NewError(http.StatusInternalServerError, "この招待コードは使用できません。")
		}

		// 招待クーポン付与
		incInvCouponCount(*req.InvitationCode)
		addUnusedCoupon(userID, 1500)
		// 招待した人にもRewardを付与
		addUnusedCoupon(inviter.ID, 1000)
	}

	createAppAccessToken(accessToken, user)
	c.Cookie(&fiber.Cookie{
		Path:  "/",
		Name:  "app_session",
		Value: accessToken,
	})
	return c.Status(http.StatusCreated).JSON(&appPostUsersResponse{
		ID:             userID,
		InvitationCode: invitationCode,
	})
}

func appPostPaymentMethods(c *fiber.Ctx) error {
	ctx := c.Context()
	req := &appPostPaymentMethodsRequest{}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest, err.Error())
	}
	if req.Token == "" {
		return fiber.NewError(http.StatusBadRequest, "token is required but was empty")
	}

	user := ctx.UserValue("user").(*User)

	createPaymentToken(user.ID, req.Token)

	return c.SendStatus(http.StatusNoContent)
}

func appGetRides(c *fiber.Ctx) error {
	ctx := c.Context()
	user := ctx.UserValue("user").(*User)

	rideIDs, _ := listRideIDsUserID(user.ID)

	items := []getAppRidesResponseItem{}
	for _, rideID := range rideIDs {
		status, _ := getLatestRideStatus(rideID)
		if status != "COMPLETED" {
			continue
		}
		ride, _ := getRide(rideID)

		item := getAppRidesResponseItem{
			ID:                    ride.ID,
			PickupCoordinate:      Coordinate{Latitude: ride.PickupLatitude, Longitude: ride.PickupLongitude},
			DestinationCoordinate: Coordinate{Latitude: ride.DestinationLatitude, Longitude: ride.DestinationLongitude},
			Fare:                  ride.Fare,
			Evaluation:            *ride.Evaluation,
			RequestedAt:           ride.CreatedAt.UnixMilli(),
			CompletedAt:           ride.UpdatedAt.UnixMilli(),
		}

		item.Chair = getAppRidesResponseItemChair{}

		chair, _ := getChair(ride.ChairID.String)
		item.Chair.ID = chair.ID
		item.Chair.Name = chair.Name
		item.Chair.Model = chair.Model

		owner, _ := getOwner(chair.OwnerID)
		item.Chair.Owner = owner.Name

		items = append(items, item)
	}

	return c.Status(http.StatusOK).JSON(&getAppRidesResponse{
		Rides: items,
	})
}

func appPostRides(c *fiber.Ctx) error {
	ctx := c.Context()
	req := &appPostRidesRequest{}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest, err.Error())
	}
	if req.PickupCoordinate == nil || req.DestinationCoordinate == nil {
		return fiber.NewError(http.StatusBadRequest, "required fields(pickup_coordinate, destination_coordinate) are empty")
	}

	user := ctx.UserValue("user").(*User)
	rideID := ulid.Make().String()

	isFree, _ := getUserRideStatus(user.ID)
	if !isFree {
		return fiber.NewError(http.StatusConflict, "ride already exists")
	}
	now := time.Now()

	discount, ok := getUnusedCoupon(user.ID)
	if ok {
		useUnusedCoupon(user.ID)
	}
	meteredFare := farePerDistance * calculateDistance(req.PickupCoordinate.Latitude, req.PickupCoordinate.Longitude, req.DestinationCoordinate.Latitude, req.DestinationCoordinate.Longitude)
	discountedMeteredFare := max(meteredFare-discount, 0)
	ride := &Ride{
		ID:                   rideID,
		UserID:               user.ID,
		PickupLatitude:       req.PickupCoordinate.Latitude,
		PickupLongitude:      req.PickupCoordinate.Longitude,
		DestinationLatitude:  req.DestinationCoordinate.Latitude,
		DestinationLongitude: req.DestinationCoordinate.Longitude,
		CreatedAt:            now,
		UpdatedAt:            now,
		Fare:                 initialFare + discountedMeteredFare,
	}
	createRide(rideID, ride)
	addRideIDsUserID(user.ID, rideID)
	waitingRides.Add(ride)

	processRideStatus(ride, "MATCHING")

	return c.Status(http.StatusAccepted).JSON(&appPostRidesResponse{
		RideID: rideID,
		Fare:   ride.Fare,
	})
}

func appPostRidesEstimatedFare(c *fiber.Ctx) error {
	ctx := c.Context()
	req := &appPostRidesEstimatedFareRequest{}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest, err.Error())
	}
	if req.PickupCoordinate == nil || req.DestinationCoordinate == nil {
		return fiber.NewError(http.StatusBadRequest, "required fields(pickup_coordinate, destination_coordinate) are empty")
	}

	user := ctx.UserValue("user").(*User)

	discount := 0
	if amount, ok := getUnusedCoupon(user.ID); ok {
		discount = amount
	}
	meteredFare := farePerDistance * calculateDistance(req.PickupCoordinate.Latitude, req.PickupCoordinate.Longitude, req.DestinationCoordinate.Latitude, req.DestinationCoordinate.Longitude)
	discountedMeteredFare := max(meteredFare-discount, 0)
	discounted := initialFare + discountedMeteredFare

	return c.Status(http.StatusOK).JSON(&appPostRidesEstimatedFareResponse{
		Fare:     discounted,
		Discount: meteredFare - discountedMeteredFare,
	})
}

func appPostRideEvaluatation(c *fiber.Ctx) error {
	ctx := c.Context()
	rideID := c.Params("ride_id")

	req := &appPostRideEvaluationRequest{}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest, err.Error())
	}
	if req.Evaluation < 1 || req.Evaluation > 5 {
		return fiber.NewError(http.StatusBadRequest, "evaluation must be between 1 and 5")
	}

	ride, ok := getRide(rideID)
	if !ok {
		return fiber.NewError(http.StatusNotFound, "ride not found")
	}
	status, _ := getLatestRideStatus(ride.ID)

	if status != "ARRIVED" {
		return fiber.NewError(http.StatusBadRequest, "not arrived yet")
	}
	if ride.ChairID.Valid {
		addChairStats(ride.ChairID.String, req.Evaluation)
	}
	ride.Evaluation = &req.Evaluation
	ride.UpdatedAt = time.Now()
	createRide(rideID, ride)

	token, ok := getPaymentToken(ride.UserID)
	if !ok {
		return fiber.NewError(http.StatusBadRequest, "payment token not registered")
	}

	paymentGatewayRequest := &paymentGatewayPostPaymentRequest{
		Amount: ride.Fare,
	}

	if err := requestPaymentGatewayPostPayment(ctx, paymentGatewayURL, rideID, token, paymentGatewayRequest); err != nil {
		if errors.Is(err, erroredUpstream) {
			return fiber.NewError(http.StatusBadGateway, err.Error())
		}
		return fiber.NewError(http.StatusInternalServerError, err.Error())
	}

	defer processRideStatus(ride, "COMPLETED")

	return c.Status(http.StatusOK).JSON(&appPostRideEvaluationResponse{
		CompletedAt: ride.UpdatedAt.UnixMilli(),
	})
}

func appGetNearbyChairs(c *fiber.Ctx) error {
	latStr := c.Query("latitude")
	lonStr := c.Query("longitude")
	distanceStr := c.Query("distance")
	if latStr == "" || lonStr == "" {
		return fiber.NewError(http.StatusBadRequest, "latitude or longitude is empty")
	}

	lat, err := strconv.Atoi(latStr)
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, "latitude is invalid")
	}

	lon, err := strconv.Atoi(lonStr)
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, "longitude is invalid")
	}

	distance := 50
	if distanceStr != "" {
		distance, err = strconv.Atoi(distanceStr)
		if err != nil {
			return fiber.NewError(http.StatusBadRequest, "distance is invalid")
		}
	}

	coordinate := Coordinate{Latitude: lat, Longitude: lon}
	nearbyChairs := []appGetNearbyChairsResponseChair{}

	freeChairs.Lock()
	retrievedAt := time.Now()
	defer freeChairs.Unlock()
	chairs := freeChairs.List()

	for _, chair := range chairs {
		chairLocation, ok := getLatestChairLocation(chair.ID)
		if !ok {
			continue
		}
		if _, ok := getLatestRide(chair.ID); ok {
			continue
		}
		if calculateDistance(coordinate.Latitude, coordinate.Longitude, chairLocation.Latitude, chairLocation.Longitude) <= distance {
			nearbyChairs = append(nearbyChairs, appGetNearbyChairsResponseChair{
				ID:    chair.ID,
				Name:  chair.Name,
				Model: chair.Model,
				CurrentCoordinate: Coordinate{
					Latitude:  chairLocation.Latitude,
					Longitude: chairLocation.Longitude,
				},
			})
		}
	}

	return c.Status(http.StatusOK).JSON(&appGetNearbyChairsResponse{
		Chairs:      nearbyChairs,
		RetrievedAt: retrievedAt.UnixMilli(),
	})
}
