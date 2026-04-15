package api

import (
	"sni-gateway/internal/services"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

type Server struct {
	app     *fiber.App
	exits   *ExitHandler
	rules   *RuleHandler
	subs    *SubscriptionHandler
	publish *PublishHandler
}

func NewServer(exitMgr *services.ExitManager, subMgr *services.SubscriptionManager, cfgGen *services.ConfigGenerator) *Server {
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": err.Error(),
			})
		},
	})

	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET,POST,PUT,DELETE,OPTIONS",
	}))

	s := &Server{
		app:     app,
		exits:   NewExitHandler(exitMgr),
		rules:   NewRuleHandler(),
		subs:    NewSubscriptionHandler(subMgr, cfgGen),
		publish: NewPublishHandler(cfgGen),
	}

	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	api := s.app.Group("/api")

	// Rules
	api.Get("/rules", s.rules.List)
	api.Post("/rules", s.rules.Create)
	api.Put("/rules/:id", s.rules.Update)
	api.Delete("/rules/:id", s.rules.Delete)

	// Subscriptions
	api.Get("/subscriptions", s.subs.List)
	api.Post("/subscriptions", s.subs.Create)
	api.Put("/subscriptions/:id", s.subs.Update)
	api.Delete("/subscriptions/:id", s.subs.Delete)
	api.Post("/subscriptions/:id/sync", s.subs.Sync)

	// Exits
	api.Get("/exits", s.exits.List)
	api.Post("/exits", s.exits.Create)
	api.Put("/exits/:id", s.exits.Update)
	api.Delete("/exits/:id", s.exits.Delete)
	api.Post("/exits/:id/start", s.exits.Start)
	api.Post("/exits/:id/stop", s.exits.Stop)
	api.Post("/exits/:id/restart", s.exits.Restart)
	api.Post("/exits/:id/test", s.exits.Test)
	api.Get("/exits/:id/config", s.exits.Config)
	api.Get("/exits/:id/logs", s.exits.Logs)
	api.Get("/exits/:id/status", s.exits.Status)

	// Publish
	api.Post("/publish/preview", s.publish.Preview)
	api.Post("/publish/validate", s.publish.Validate)
	api.Post("/publish/apply", s.publish.Apply)
	api.Post("/publish/rollback", s.publish.Rollback)
	api.Get("/publish/history", s.publish.History)

	// Health check
	s.app.Get("/health", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	// Dashboard stats
	api.Get("/stats", s.dashboardStats)

	// Static files for frontend
	s.app.Static("/", "./public")
	s.app.Get("/*", func(c *fiber.Ctx) error {
		return c.SendFile("./public/index.html")
	})
}

func (s *Server) dashboardStats(c *fiber.Ctx) error {
	return getDashboardStats(c)
}

func (s *Server) Listen(addr string) error {
	return s.app.Listen(addr)
}
