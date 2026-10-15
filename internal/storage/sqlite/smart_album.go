package sqlite

import (
	"fmt"
	"strings"

	"echogallery/internal/storage"
)

func smartAlbumPredicate(alias, matchMode string, rules []storage.SmartAlbumRule) (string, []interface{}, error) {
	if alias != "p" && alias != "p2" {
		return "", nil, fmt.Errorf("智能相册查询别名无效")
	}
	mode, normalized, err := storage.NormalizeSmartAlbumDefinition(matchMode, rules)
	if err != nil {
		return "", nil, err
	}
	conditions := make([]string, 0, len(normalized))
	args := make([]interface{}, 0, len(normalized)*2)
	for _, rule := range normalized {
		switch rule.Field {
		case "media_type":
			operator := "="
			if rule.Operator == "is_not" {
				operator = "<>"
			}
			conditions = append(conditions, alias+".media_kind "+operator+" ?")
			args = append(args, rule.Value)
		case "duration":
			condition := "(" + alias + ".media_kind = 'video' AND "
			switch rule.Operator {
			case "between":
				min, _ := storage.ParseSmartAlbumDuration(rule.Min)
				max, _ := storage.ParseSmartAlbumDuration(rule.Max)
				condition += alias + ".duration_ms >= ? AND " + alias + ".duration_ms <= ?)"
				args = append(args, min, max)
			case "at_least":
				min, _ := storage.ParseSmartAlbumDuration(rule.Min)
				condition += alias + ".duration_ms >= ?)"
				args = append(args, min)
			case "at_most":
				max, _ := storage.ParseSmartAlbumDuration(rule.Max)
				condition += alias + ".duration_ms <= ?)"
				args = append(args, max)
			}
			conditions = append(conditions, condition)
		case "filename":
			name := "lower(COALESCE(" + alias + ".original_name, ''))"
			value := strings.ToLower(rule.Value)
			switch rule.Operator {
			case "contains":
				conditions = append(conditions, "instr("+name+", lower(?)) > 0")
				args = append(args, value)
			case "not_contains":
				conditions = append(conditions, "instr("+name+", lower(?)) = 0")
				args = append(args, value)
			case "starts_with":
				conditions = append(conditions, "substr("+name+", 1, length(?)) = lower(?)")
				args = append(args, value, value)
			case "ends_with":
				conditions = append(conditions, "substr("+name+", -length(?)) = lower(?)")
				args = append(args, value, value)
			}
		}
	}
	joiner := " AND "
	if mode == "any" {
		joiner = " OR "
	}
	return "(" + strings.Join(conditions, joiner) + ")", args, nil
}

func (s *DB) refreshSmartAlbumStats(album *storage.Album) error {
	if album == nil || album.SourceKind != "smart" {
		return nil
	}
	predicate, ruleArgs, err := smartAlbumPredicate("p", album.SmartMatchMode, album.SmartRules)
	if err != nil {
		return fmt.Errorf("智能相册规则无效: %w", err)
	}
	coverID := int64(0)
	if album.CoverPhotoID != nil {
		coverID = *album.CoverPhotoID
	}
	args := []interface{}{coverID, album.CreatedBy, album.CreatedBy}
	args = append(args, ruleArgs...)
	args = append(args, album.CreatedBy)
	args = append(args, ruleArgs...)
	var coverUUID string
	var count int
	err = s.db.QueryRow(`
		SELECT COUNT(*), COALESCE(
			(SELECT ph.uuid FROM photos ph
			 WHERE ph.id = ? AND ph.uploaded_by = ? AND ph.deleted_at IS NULL LIMIT 1),
			(SELECT p2.uuid FROM photos p2
			 WHERE p2.uploaded_by = ? AND p2.deleted_at IS NULL AND `+strings.ReplaceAll(predicate, "p.", "p2.")+`
			 ORDER BY p2.taken_at DESC, p2.id DESC LIMIT 1),
			''
		)
		FROM photos p
		WHERE p.uploaded_by = ? AND p.deleted_at IS NULL AND `+predicate, args...).Scan(&count, &coverUUID)
	if err != nil {
		return fmt.Errorf("统计智能相册媒体失败: %w", err)
	}
	album.PhotoCount = count
	album.CoverUUID = coverUUID
	return nil
}
