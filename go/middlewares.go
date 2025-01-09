package main

import (
	"context"
	"errors"
	"net/http"

	"github.com/gofiber/fiber/v2"
)

func appAuthMiddlewareFiber(c *fiber.Ctx) error {
	ctx := c.Context()
	accessToken := c.Cookies("app_session")
	if accessToken == "" {
		return fiber.NewError(http.StatusUnauthorized, "app_session cookie is required")
	}
	user, ok := getAppAccessToken(accessToken)
	if !ok {
		return fiber.NewError(http.StatusUnauthorized, "invalid access token")
	}
	// ctx = context.WithValue(ctx, "user", user)
	ctx.SetUserValue("user", user)
	// next.ServeHTTP(w, r.WithContext(ctx))
	return c.Next()
}

func ownerAuthMiddlewareFiber(c *fiber.Ctx) error {
	ctx := c.Context()
	accessToken := c.Cookies("owner_session")
	if accessToken == "" {
		return fiber.NewError(http.StatusUnauthorized, "owner_session cookie is required")
	}
	owner, ok := getOwnerAccessToken(accessToken)
	if !ok {
		return fiber.NewError(http.StatusUnauthorized, "invalid access token")
	}

	// ctx = context.WithValue(ctx, "owner", owner)
	ctx.SetUserValue("owner", owner)
	// next.ServeHTTP(w, r.WithContext(ctx))
	return c.Next()
}

func chairAuthMiddlewareFiber(c *fiber.Ctx) error {
	ctx := c.Context()
	accessToken := c.Cookies("chair_session")
	if accessToken == "" {
		return fiber.NewError(http.StatusUnauthorized, "chair_session cookie is required")
	}
	chair, ok := getChairAccessToken(accessToken)
	if !ok {
		return fiber.NewError(http.StatusUnauthorized, "invalid access token")
	}

	// ctx = context.WithValue(ctx, "chair", chair)
	ctx.SetUserValue("chair", chair)
	// next.ServeHTTP(w, r.WithContext(ctx))
	return c.Next()
}

func appAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		c, err := r.Cookie("app_session")
		if errors.Is(err, http.ErrNoCookie) || c.Value == "" {
			writeError(w, http.StatusUnauthorized, errors.New("app_session cookie is required"))
			return
		}
		accessToken := c.Value
		user, ok := getAppAccessToken(accessToken)
		if !ok {
			writeError(w, http.StatusUnauthorized, errors.New("invalid access token"))
			return
		}
		ctx = context.WithValue(ctx, "user", user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func ownerAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		c, err := r.Cookie("owner_session")
		if errors.Is(err, http.ErrNoCookie) || c.Value == "" {
			writeError(w, http.StatusUnauthorized, errors.New("owner_session cookie is required"))
			return
		}
		accessToken := c.Value
		owner, ok := getOwnerAccessToken(accessToken)
		if !ok {
			writeError(w, http.StatusUnauthorized, errors.New("invalid access token"))
			return
		}

		ctx = context.WithValue(ctx, "owner", owner)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func chairAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		c, err := r.Cookie("chair_session")
		if errors.Is(err, http.ErrNoCookie) || c.Value == "" {
			writeError(w, http.StatusUnauthorized, errors.New("chair_session cookie is required"))
			return
		}
		accessToken := c.Value
		chair, ok := getChairAccessToken(accessToken)
		if !ok {
			writeError(w, http.StatusUnauthorized, errors.New("invalid access token"))
			return
		}

		ctx = context.WithValue(ctx, "chair", chair)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
