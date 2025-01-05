package main

import (
	"bufio"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v2"
	"github.com/oklog/ulid/v2"
	"github.com/valyala/fasthttp"
)

func appPostUsersFiber(c *fiber.Ctx) error {
	req := &appPostUsersRequest{}
	if err := c.BodyParser(req); err != nil {
		return c.SendStatus(fiber.StatusBadRequest)
	}
	if req.Username == "" || req.FirstName == "" || req.LastName == "" || req.DateOfBirth == "" {
		return c.SendStatus(fiber.StatusBadRequest)
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
			return c.SendStatus(fiber.StatusInternalServerError)
		}

		// ユーザーチェック
		inviter, ok := getUserInv(*req.InvitationCode)
		if !ok {
			return c.SendStatus(fiber.StatusInternalServerError)
		}

		// 招待クーポン付与
		incInvCouponCount(*req.InvitationCode)
		addUnusedCoupon(userID, 1500)
		// 招待した人にもRewardを付与
		addUnusedCoupon(inviter.ID, 1000)
	}

	createAppAccessToken(accessToken, user)
	c.Cookie(&fiber.Cookie{
		// Path:  "/",
		Name:  "app_session",
		Value: accessToken,
	})
	return c.Status(fiber.StatusCreated).JSON(&appPostUsersResponse{
		ID:             userID,
		InvitationCode: invitationCode,
	})
}

func appPostPaymentMethodsFiber(c *fiber.Ctx) error {
	req := &appPostPaymentMethodsRequest{}
	if err := c.BodyParser(req); err != nil {
		return c.SendStatus(fiber.StatusBadRequest)
	}
	if req.Token == "" {
		return c.SendStatus(fiber.StatusBadRequest)
	}

	user := c.Locals("user").(*User)

	createPaymentToken(user.ID, req.Token)

	return c.SendStatus(fiber.StatusNoContent)
}

func appGetRidesFiber(c *fiber.Ctx) error {
	user := c.Locals("user").(*User)

	rideIDs, _ := listRideIDsUserID(user.ID)

	items := []getAppRidesResponseItem{}
	for _, rideID := range rideIDs {
		status, _ := getLatestRideStatus(rideID)
		if status != "COMPLETED" {
			continue
		}
		ride, _ := getRide(rideID)
		fare := calculateDiscountedFare(user.ID, ride, ride.PickupLatitude, ride.PickupLongitude, ride.DestinationLatitude, ride.DestinationLongitude)

		item := getAppRidesResponseItem{
			ID:                    ride.ID,
			PickupCoordinate:      Coordinate{Latitude: ride.PickupLatitude, Longitude: ride.PickupLongitude},
			DestinationCoordinate: Coordinate{Latitude: ride.DestinationLatitude, Longitude: ride.DestinationLongitude},
			Fare:                  fare,
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

	return c.JSON(&getAppRidesResponse{
		Rides: items,
	})
}

func appPostRidesFiber(c *fiber.Ctx) error {
	req := &appPostRidesRequest{}
	if err := c.BodyParser(req); err != nil {
		return c.SendStatus(fiber.StatusBadRequest)
	}
	// if req.PickupCoordinate == nil || req.DestinationCoordinate == nil {
	// 	return c.SendStatus(fiber.StatusBadRequest)
	// }

	user := c.Locals("user").(*User)
	rideID := ulid.Make().String()

	isFree, _ := getUserRideStatus(user.ID)
	if !isFree {
		return c.SendStatus(fiber.StatusConflict)
	}
	now := time.Now()
	ride := &Ride{
		ID:                   rideID,
		UserID:               user.ID,
		PickupLatitude:       req.PickupCoordinate.Latitude,
		PickupLongitude:      req.PickupCoordinate.Longitude,
		DestinationLatitude:  req.DestinationCoordinate.Latitude,
		DestinationLongitude: req.DestinationCoordinate.Longitude,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	createRide(rideID, ride)
	addRideIDsUserID(user.ID, ride.ID)
	waitingRides.Add(ride)

	// 初回利用クーポンは初回に必ず使われるしこれだけでok
	if amount, ok := getUnusedCoupon(user.ID); ok {
		useUnusedCoupon(user.ID)
		createRideDiscount(rideID, amount)
	}

	fare := calculateDiscountedFare(user.ID, ride, req.PickupCoordinate.Latitude, req.PickupCoordinate.Longitude, req.DestinationCoordinate.Latitude, req.DestinationCoordinate.Longitude)

	processRideStatus(ride, "MATCHING")

	return c.Status(fiber.StatusAccepted).JSON(&appPostRidesResponse{
		RideID: rideID,
		Fare:   fare,
	})
}

func appPostRidesEstimatedFareFiber(c *fiber.Ctx) error {
	req := &appPostRidesEstimatedFareRequest{}
	if err := c.BodyParser(req); err != nil {
		return c.SendStatus(fiber.StatusBadRequest)
	}
	if req.PickupCoordinate == nil || req.DestinationCoordinate == nil {
		return c.SendStatus(fiber.StatusBadRequest)
	}

	user := c.Locals("user").(*User)

	discounted := calculateDiscountedFare(user.ID, nil, req.PickupCoordinate.Latitude, req.PickupCoordinate.Longitude, req.DestinationCoordinate.Latitude, req.DestinationCoordinate.Longitude)

	return c.JSON(&appPostRidesEstimatedFareResponse{
		Fare:     discounted,
		Discount: calculateFare(req.PickupCoordinate.Latitude, req.PickupCoordinate.Longitude, req.DestinationCoordinate.Latitude, req.DestinationCoordinate.Longitude) - discounted,
	})
}

func appPostRideEvaluatationFiber(c *fiber.Ctx) error {
	rideID := c.Params("ride_id")

	req := &appPostRideEvaluationRequest{}
	if err := c.BodyParser(req); err != nil {
		return c.SendStatus(fiber.StatusBadRequest)
	}
	if req.Evaluation < 1 || req.Evaluation > 5 {
		return c.SendStatus(fiber.StatusBadRequest)
	}

	ride, ok := getRide(rideID)
	if !ok {
		return c.SendStatus(fiber.StatusNotFound)
	}
	status, _ := getLatestRideStatus(ride.ID)

	if status != "ARRIVED" {
		return c.SendStatus(fiber.StatusBadRequest)
	}
	if ride.ChairID.Valid {
		addChairStats(ride.ChairID.String, req.Evaluation)
	}
	ride.Evaluation = &req.Evaluation
	ride.UpdatedAt = time.Now()
	createRide(rideID, ride)

	token, ok := getPaymentToken(ride.UserID)
	if !ok {
		return c.SendStatus(fiber.StatusBadRequest)
	}

	fare := calculateDiscountedFare(ride.UserID, ride, ride.PickupLatitude, ride.PickupLongitude, ride.DestinationLatitude, ride.DestinationLongitude)
	paymentGatewayRequest := &paymentGatewayPostPaymentRequest{
		Amount: fare,
	}

	if err := requestPaymentGatewayPostPayment(c.Context(), paymentGatewayURL, rideID, token, paymentGatewayRequest); err != nil {
		if errors.Is(err, erroredUpstream) {
			return c.SendStatus(fiber.StatusBadGateway)

		}
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	defer processRideStatus(ride, "COMPLETED")

	return c.JSON(&appPostRideEvaluationResponse{
		CompletedAt: ride.UpdatedAt.UnixMilli(),
	})
}

// SSE
func appGetNotificationFiber(c *fiber.Ctx) error {
	user := c.Locals("user").(*User)

	c.Set("X-Accel-Buffering", "no")
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	clientGone := c.Context().Done()
	appChan := getAppChan(user.ID)
	c.Status(fiber.StatusOK).Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
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
				// if _, err := w.Write([]byte("data: ")); err != nil {
				// 	return
				// }
				// if _, err := w.Write(resV); err != nil {
				// 	return
				// }
				// if _, err := w.Write([]byte("\n\n")); err != nil {
				// 	return
				// }
				fmt.Fprintf(w, "data: %s\n\n", string(resV))
				// fmt.Printf("[DEBUG] Fprintf %s\n", string(resV))
				if err := w.Flush(); err != nil {
					return
				}
				if notif.RideStatus == "COMPLETED" {
					deleteLatestRide(notif.Ride.ChairID.String)
				}
			}
		}
	}))

	return nil
}

func appGetNearbyChairsFiber(c *fiber.Ctx) error {
	latStr := c.Query("latitude")
	lonStr := c.Query("longitude")
	distanceStr := c.Query("distance")
	if latStr == "" || lonStr == "" {
		return c.SendStatus(fiber.StatusBadRequest)
	}

	lat, err := strconv.Atoi(latStr)
	if err != nil {
		return c.SendStatus(fiber.StatusBadRequest)
	}

	lon, err := strconv.Atoi(lonStr)
	if err != nil {
		return c.SendStatus(fiber.StatusBadRequest)
	}

	distance := 50
	if distanceStr != "" {
		distance, err = strconv.Atoi(distanceStr)
		if err != nil {
			return c.SendStatus(fiber.StatusBadRequest)
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

	return c.JSON(&appGetNearbyChairsResponse{
		Chairs:      nearbyChairs,
		RetrievedAt: retrievedAt.UnixMilli(),
	})
}
