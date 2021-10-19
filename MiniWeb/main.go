package main

import (
	log "github.com/sirupsen/logrus"
	"time"
)

func main() {
	app := fiber.New(fiber.Config{
		Prefork:              false,
		ServerHeader:         "",
		StrictRouting:        false,
		CaseSensitive:        true,
		Immutable:            false,
		UnescapePath:         true,
		ETag:                 false,
		BodyLimit:            0,
		Concurrency:          0,
		Views:                nil,
		ViewsLayout:          "",
		ReadTimeout:          time.Minute * 5,
		WriteTimeout:         time.Minute * 5,
		IdleTimeout:          0,
		ReadBufferSize:       0,
		WriteBufferSize:      0,
		CompressedFileSuffix: ".gz",
		ProxyHeader:          "",
		GETOnly:              false,
		ErrorHandler: func(ctx *fiber.Ctx, err error) error {
			return ctx.Status(500).SendString(err.Error())
		},
		DisableKeepalive:          false,
		DisableDefaultDate:        false,
		DisableDefaultContentType: false,
		DisableHeaderNormalizing:  false,
		DisableStartupMessage:     true,
		ReduceMemoryUsage:         true,
		JSONEncoder:               nil,
		Network:                   "",
	})

	app.All("*", func(ctx *fiber.Ctx) error {
		log.Info(ctx.String())
		log.Info(string(ctx.Body()))

		log.Info(ctx.Request().Header)
		return ctx.SendStatus(200)
	})

	err := app.Listen("0.0.0.0:8031")
	if err != nil {
		log.Errorf("err:%v", err)
		return
	}
}
