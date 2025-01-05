package main

import (
	"github.com/gofiber/fiber/v2"
)

func appAuthMiddlewareFiber(c *fiber.Ctx) error {
	accessToken := c.Cookies("app_session")
	if accessToken == "" {
		return c.Status(fiber.StatusUnauthorized).SendString("app_session cookie is required")
	}
	user, ok := getAppAccessToken(accessToken)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).SendString("invalid access token")
	}
	c.Locals("user", user)
	return c.Next()
}

func ownerAuthMiddlewareFiber(c *fiber.Ctx) error {
	accessToken := c.Cookies("owner_session")
	if accessToken == "" {
		return c.Status(fiber.StatusUnauthorized).SendString("owner_session cookie is required")
	}
	owner, ok := getOwnerAccessToken(accessToken)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).SendString("invalid access token")
	}
	c.Locals("owner", owner)
	return c.Next()
}

func chairAuthMiddlewareFiber(c *fiber.Ctx) error {
	accessToken := c.Cookies("chair_session")
	if accessToken == "" {
		return c.Status(fiber.StatusUnauthorized).SendString("chair_session cookie is required")
	}
	chair, ok := getChairAccessToken(accessToken)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).SendString("invalid access token")
	}
	c.Locals("chair", chair)
	return c.Next()
}
