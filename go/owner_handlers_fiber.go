package main

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/oklog/ulid/v2"
)

func ownerPostOwnersFiber(c *fiber.Ctx) error {
	req := &ownerPostOwnersRequest{}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest, err.Error())
	}
	if req.Name == "" {
		return fiber.NewError(http.StatusBadRequest, "some of required fields(name) are empty")
	}

	ownerID := ulid.Make().String()
	accessToken := secureRandomStr(32)
	chairRegisterToken := secureRandomStr(32)
	now := time.Now()
	owner := &Owner{
		ID:                 ownerID,
		Name:               req.Name,
		AccessToken:        accessToken,
		ChairRegisterToken: chairRegisterToken,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	createOwnerAccessToken(accessToken, owner)
	createOwner(ownerID, owner)
	createOwnerChairRegisterToken(chairRegisterToken, owner)

	c.Cookie(&fiber.Cookie{
		Path:  "/",
		Name:  "owner_session",
		Value: accessToken,
	})

	return c.Status(http.StatusCreated).JSON(&ownerPostOwnersResponse{
		ID:                 ownerID,
		ChairRegisterToken: chairRegisterToken,
	})
}

func ownerGetSalesFiber(c *fiber.Ctx) error {
	since := time.Unix(0, 0)
	until := time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
	if c.Query("since") != "" {
		parsed, err := strconv.ParseInt(c.Query("since"), 10, 64)
		if err != nil {
			return fiber.NewError(http.StatusBadRequest, err.Error())
		}
		since = time.UnixMilli(parsed)
	}
	if c.Query("until") != "" {
		parsed, err := strconv.ParseInt(c.Query("until"), 10, 64)
		if err != nil {
			return fiber.NewError(http.StatusBadRequest, err.Error())
		}
		until = time.UnixMilli(parsed)
	}

	owner := c.Context().UserValue("owner").(*Owner)

	chairs, _ := getChairsOwnerID(owner.ID)
	res := ownerGetSalesResponse{
		TotalSales: 0,
	}

	modelSalesByModel := map[string]int{}
	for _, chair := range chairs {
		if _, ok := modelSalesByModel[chair.Model]; !ok {
			modelSalesByModel[chair.Model] = 0
		}
		sales, ok := getChairSale(chair.ID)
		if !ok {
			res.Chairs = append(res.Chairs, chairSales{
				ID:    chair.ID,
				Name:  chair.Name,
				Sales: 0,
			})
			continue
		}
		sumSales := 0
		for _, sale := range sales {
			if sale.UpdatedAt.Before(since) || sale.UpdatedAt.After(until.Add(999*time.Microsecond)) {
				continue
			}
			sumSales += sale.Sale
		}
		res.Chairs = append(res.Chairs, chairSales{
			ID:    chair.ID,
			Name:  chair.Name,
			Sales: sumSales,
		})
		res.TotalSales += sumSales
		modelSalesByModel[chair.Model] += sumSales
	}

	models := []modelSales{}
	for model, sales := range modelSalesByModel {
		models = append(models, modelSales{
			Model: model,
			Sales: sales,
		})
	}
	res.Models = models
	return c.Status(http.StatusOK).JSON(res)
}

func ownerGetChairsFiber(c *fiber.Ctx) error {
	ctx := c.Context()
	owner := ctx.UserValue("owner").(*Owner)

	chairs, _ := getChairsOwnerID(owner.ID)
	res := ownerGetChairResponse{}
	for _, chair := range chairs {
		current, ok := getChairTotalDistance(chair.ID)
		c := ownerGetChairResponseChair{
			ID:           chair.ID,
			Name:         chair.Name,
			Model:        chair.Model,
			Active:       chair.IsActive, // 初回以降更新してないのになぜか通る
			RegisteredAt: chair.CreatedAt.UnixMilli(),
		}
		if ok {
			temp := current.UpdatedAt.UnixMilli()
			c.TotalDistanceUpdatedAt = &temp
			c.TotalDistance = current.TotalDistance
		}
		res.Chairs = append(res.Chairs, c)
	}
	return c.Status(http.StatusOK).JSON(res)
}
