package main

import (
	"fmt"
	"strings"

	"github.com/finb/bark-server/apns"

	"github.com/gofiber/fiber/v2"

	"go.etcd.io/bbolt"
)

func init() {
	registerRoute("push", func(router *fiber.App) {
		router.Post("/push", func(c *fiber.Ctx) error { return routeDoPush(c, false) })
	})

	// compatible with old requests
	registerRouteWithWeight("push_compat", 1, func(router *fiber.App) {
		router.Get("/:device_key/:body", func(c *fiber.Ctx) error { return routeDoPush(c, true) })
		router.Post("/:device_key/:body", func(c *fiber.Ctx) error { return routeDoPush(c, true) })

		router.Get("/:device_key/:title/:body", func(c *fiber.Ctx) error { return routeDoPush(c, true) })
		router.Post("/:device_key/:title/:body", func(c *fiber.Ctx) error { return routeDoPush(c, true) })

		router.Get("/:device_key/:category/:title/:body", func(c *fiber.Ctx) error { return routeDoPush(c, true) })
		router.Post("/:device_key/:category/:title/:body", func(c *fiber.Ctx) error { return routeDoPush(c, true) })
	})
}

func routeDoPush(c *fiber.Ctx, compat bool) error {
	var deviceKey string
	// default value
	msg := apns.PushMessage{
		Category:  "Bark",
		Body:      "NoContent",
		Sound:     "1107",
		ExtParams: map[string]string{},
	}

	// always parse body(Lowest priority)
	if err := c.BodyParser(&msg); err != nil && err != fiber.ErrUnprocessableEntity {
		return c.Status(400).JSON(failed(400, "request bind failed: %v", err))
	}

	if compat {
		// parse query args (medium priority)
		c.Request().URI().QueryArgs().VisitAll(func(key, value []byte) {
			switch strings.ToLower(string(key)) {
			case "device_key":
				deviceKey = string(value)
			case "category":
				msg.Category = string(value)
			case "title":
				msg.Title = string(value)
			case "body":
				msg.Body = string(value)
			case "sound":
				msg.Sound = string(value) + ".caf"
			default:
				msg.ExtParams[strings.ToLower(string(key))] = string(value)
			}
		})

		// parse url path (highest priority)
		if pathDeviceKey := c.Params("device_key"); pathDeviceKey != "" {
			deviceKey = pathDeviceKey
		}
		if category := c.Params("category"); category != "" {
			msg.Category = category
		}
		if title := c.Params("title"); title != "" {
			msg.Title = title
		}
		if body := c.Params("body"); body != "" {
			msg.Body = body
		}
	}

	if deviceKey == "" {
		return c.Status(400).JSON(failed(400, "device key is empty"))
	}

	err := db.View(func(tx *bbolt.Tx) error {
		if bs := tx.Bucket([]byte(bucketName)).Get([]byte(deviceKey)); bs == nil {
			return fmt.Errorf("failed to get [%s] device token from database", deviceKey)
		} else {
			msg.DeviceToken = string(bs)
			return nil
		}
	})
	if err != nil {
		return c.Status(400).JSON(failed(400, "failed to get device token: %v", err))
	}

	err = apns.Push(&msg)
	if err != nil {
		return c.Status(500).JSON(failed(500, "push failed: %v", err))
	}
	return c.JSON(success())
}
