package main

import (
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
