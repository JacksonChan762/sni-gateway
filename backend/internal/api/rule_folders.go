package api

import (
	"errors"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"sni-gateway/internal/db"
	"sni-gateway/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

func listRuleFolders(c *fiber.Ctx) error {
	folders := []models.RuleFolder{}
	if err := db.DB.Order("name ASC, id ASC").Find(&folders).Error; err != nil {
		return err
	}
	return c.JSON(folders)
}

func createRuleFolder(c *fiber.Ctx) error {
	name, err := decodeRuleFolderName(c.Body())
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	folder := models.RuleFolder{Name: name}
	if err := db.DB.Create(&folder).Error; err != nil {
		return ruleFolderWriteError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(folder)
}

func updateRuleFolder(c *fiber.Ctx) error {
	id, ok := parseRuleFolderID(c.Params("id"))
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "folder ID must be a positive integer"})
	}
	name, err := decodeRuleFolderName(c.Body())
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	folder := models.RuleFolder{ID: id, Name: name}
	result := db.DB.Model(&folder).Update("name", name)
	if result.Error != nil {
		return ruleFolderWriteError(c, result.Error)
	}
	if result.RowsAffected == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "folder not found"})
	}
	return c.JSON(folder)
}

func deleteRuleFolder(c *fiber.Ctx) error {
	id, ok := parseRuleFolderID(c.Params("id"))
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "folder ID must be a positive integer"})
	}
	result := db.DB.Delete(&models.RuleFolder{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "folder not found"})
	}
	return c.JSON(fiber.Map{"message": "deleted"})
}

func parseRuleFolderID(raw string) (uint, bool) {
	id, err := strconv.ParseUint(raw, 10, min(63, strconv.IntSize))
	return uint(id), err == nil && id > 0
}

func validOptionalRuleFolderID(id *uint) bool {
	return id == nil || (*id > 0 && uint64(*id) <= (1<<63)-1)
}

func decodeRuleFolderName(body []byte) (string, error) {
	var req struct {
		Name string `json:"name"`
	}
	if _, err := decodeJSONBody(body, &req); err != nil {
		return "", err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || utf8.RuneCountInString(name) > 100 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return "", errors.New("folder name must be 1–100 characters without control characters")
	}
	return name, nil
}

func ruleFolderWriteError(c *fiber.Ctx, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "a folder with this name already exists"})
	}
	return err
}

func ensureRuleFoldersExist(rules []models.Rule) error {
	unique := make(map[uint]struct{})
	for _, rule := range rules {
		if rule.FolderID != nil {
			unique[*rule.FolderID] = struct{}{}
		}
	}
	if len(unique) == 0 {
		return nil
	}
	ids := make([]uint, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}
	var count int64
	if err := db.DB.Model(&models.RuleFolder{}).Where("id IN ?", ids).Count(&count).Error; err != nil {
		return err
	}
	if count != int64(len(ids)) {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func ruleFolderReferenceResponse(c *fiber.Ctx, err error) error {
	var pgErr *pgconn.PgError
	// The foreign key also catches a folder deleted after the existence check.
	if errors.Is(err, gorm.ErrRecordNotFound) ||
		(errors.As(err, &pgErr) && pgErr.Code == "23503" && pgErr.ConstraintName == "fk_rules_folder") {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "folder_id does not reference an existing folder"})
	}
	return err
}
