package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/bytedance/sonic"
)

var erroredUpstream = errors.New("errored upstream")

type paymentGatewayPostPaymentRequest struct {
	Amount int `json:"amount"`
}

type paymentGatewayGetPaymentsResponseOne struct {
	Amount int    `json:"amount"`
	Status string `json:"status"`
}

func requestPaymentGatewayPostPayment(ctx context.Context, paymentGatewayURL string, rideId, token string, param *paymentGatewayPostPaymentRequest) error {
	b, err := sonic.Marshal(param)
	if err != nil {
		return err
	}

	retry := 0
	for {
		err := func() error {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, paymentGatewayURL+"/payments", bytes.NewBuffer(b))
			if err != nil {
				return err
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Idempotency-Key", rideId)

			res, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}
			defer res.Body.Close()

			if res.StatusCode != http.StatusNoContent {
				return fmt.Errorf("unexpected status code (%d)", res.StatusCode)
			}
			return nil
		}()
		if err != nil {
			if retry < 5 {
				retry++
				continue
			} else {
				return err
			}
		}
		break
	}

	return nil
}
