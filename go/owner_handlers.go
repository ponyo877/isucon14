package main

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/oklog/ulid/v2"
)

func ownerPostOwners(w http.ResponseWriter, r *http.Request) {
	req := &ownerPostOwnersRequest{}
	if err := bindJSON(r, req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, errors.New("some of required fields(name) are empty"))
		return
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

	http.SetCookie(w, &http.Cookie{
		Path:  "/",
		Name:  "owner_session",
		Value: accessToken,
	})

	writeJSON(w, http.StatusCreated, &ownerPostOwnersResponse{
		ID:                 ownerID,
		ChairRegisterToken: chairRegisterToken,
	})
}

func ownerGetSales(w http.ResponseWriter, r *http.Request) {
	since := time.Unix(0, 0)
	until := time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
	if r.URL.Query().Get("since") != "" {
		parsed, err := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		since = time.UnixMilli(parsed)
	}
	if r.URL.Query().Get("until") != "" {
		parsed, err := strconv.ParseInt(r.URL.Query().Get("until"), 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		until = time.UnixMilli(parsed)
	}

	owner := r.Context().Value("owner").(*Owner)

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
	writeJSON(w, http.StatusOK, res)
}

func ownerGetChairs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	owner := ctx.Value("owner").(*Owner)

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
	writeJSON(w, http.StatusOK, res)
}
