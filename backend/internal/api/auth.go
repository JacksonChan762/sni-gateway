package api

import (
	"crypto/subtle"
	"strings"
	"time"

	jwtware "github.com/gofiber/contrib/jwt"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

// AuthMiddleware validates JWT tokens on protected routes.
func AuthMiddleware(secret string) fiber.Handler {
	return jwtware.New(jwtware.Config{
		SigningKey: jwtware.SigningKey{Key: []byte(secret)},
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "unauthorized",
			})
		},
	})
}

type LoginHandler struct {
	secret   string
	username string
	password string
}

func NewLoginHandler(secret, username, password string) *LoginHandler {
	return &LoginHandler{secret: secret, username: username, password: password}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *LoginHandler) Login(c *fiber.Ctx) error {
	var req loginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		return c.Status(400).JSON(fiber.Map{"error": "username and password are required"})
	}
	usernameMatches := subtle.ConstantTimeCompare([]byte(req.Username), []byte(h.username)) == 1
	passwordMatches := subtle.ConstantTimeCompare([]byte(req.Password), []byte(h.password)) == 1
	if !usernameMatches || !passwordMatches {
		return c.Status(401).JSON(fiber.Map{"error": "invalid credentials"})
	}

	now := time.Now()
	expiresAt := now.Add(24 * time.Hour)
	claims := jwt.MapClaims{
		"sub": req.Username,
		"exp": expiresAt.Unix(),
		"iat": now.Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(h.secret))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token generation failed"})
	}

	return c.JSON(fiber.Map{
		"token":      signed,
		"username":   req.Username,
		"expires_at": expiresAt.Unix(),
	})
}
