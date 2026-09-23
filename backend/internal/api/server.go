package api

import (
	"sni-gateway/internal/config"
	"sni-gateway/internal/services"
	"time"

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
	login   *LoginHandler
	cfg     *config.Config
}

func NewServer(cfg *config.Config, exitMgr *services.ExitManager, subMgr *services.SubscriptionManager, cfgGen services.Publisher) *Server {
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
		exits:   NewExitHandler(exitMgr, cfgGen),
		rules:   NewRuleHandler(cfgGen),
		subs:    NewSubscriptionHandler(subMgr, cfgGen),
		publish: NewPublishHandler(cfgGen),
		login:   NewLoginHandler(cfg.JWTSecret, cfg.AdminUsername, cfg.AdminPassword),
		cfg:     cfg,
	}

	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	// Health check (public)
	s.app.Get("/health", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	// Auth endpoint (public)
	s.app.Post("/api/auth/login", s.login.Login)

	// All other /api routes require a valid JWT
	api := s.app.Group("/api", AuthMiddleware(s.cfg.JWTSecret))

	// Stats
	api.Get("/stats", s.dashboardStats)

	// Rules
	api.Get("/rules", s.rules.List)
	api.Post("/rules", s.rules.Create)
	api.Post("/rules/test", s.rules.Test)
	api.Post("/rules/bulk", s.rules.CreateBulk)
	api.Put("/rules/:id", s.rules.Update)
	api.Delete("/rules/:id", s.rules.Delete)

	// Subscriptions
	api.Get("/subscriptions", s.subs.List)
	api.Post("/subscriptions", s.subs.Create)
	api.Put("/subscriptions/:id", s.subs.Update)
	api.Delete("/subscriptions/:id", s.subs.Delete)
	api.Post("/subscriptions/:id/sync", s.subs.Sync)

	// Exits
	api.Get("/providers", listVPNProviders)
	api.Get("/providers/locations", listVPNLocations)
	api.Get("/providers/servers", searchVPNServers)
	api.Get("/exits", s.exits.List)
	api.Post("/exits", s.exits.Create)
	api.Post("/exits/stop-all", s.exits.StopAll)
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
	api.Get("/publish/status", s.publish.Status)
	api.Post("/publish/preview", s.publish.Preview)
	api.Post("/publish/validate", s.publish.Validate)
	api.Post("/publish/apply", s.publish.Apply)
	api.Post("/publish/rollback", s.publish.Rollback)
	api.Get("/publish/history", s.publish.History)

	// Static files for frontend (must be last)
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

func (s *Server) Shutdown() error { return s.app.ShutdownWithTimeout(15 * time.Second) }
