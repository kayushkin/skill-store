package skillstore

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ErrInstallConflict is returned when a different skill is already installed
// at the target path. Uninstall the existing one first (or retry with force).
var ErrInstallConflict = errors.New("another skill is already installed at this target")

// Installation mirrors the installations table for API responses.
type Installation struct {
	SkillID     int64  `json:"skill_id"`
	SkillName   string `json:"skill_name,omitempty"`
	SourceName  string `json:"source_name,omitempty"`
	TargetPath  string `json:"target_path"`
	InstalledAt int64  `json:"installed_at"`
}

// Install copies the skill's SKILL.md plus sibling files into
// <installDir>/<name>/. If another skill already occupies that target path, it
// returns ErrInstallConflict unless force is true (in which case the previous
// skill is uninstalled first).
func (s *Store) Install(skillID int64, force bool) (*Installation, error) {
	sk, err := s.GetSkill(skillID)
	if err != nil {
		return nil, err
	}
	target := filepath.Join(s.installDir, sk.Name)

	// Check for another installation at this target path.
	var existingSkillID int64
	err = s.db.QueryRow(`SELECT skill_id FROM installations WHERE target_path=?`, target).Scan(&existingSkillID)
	if err == nil && existingSkillID != skillID {
		if !force {
			return nil, fmt.Errorf("%w: skill_id=%d owns %s", ErrInstallConflict, existingSkillID, target)
		}
		if _, err := s.Uninstall(existingSkillID); err != nil {
			return nil, fmt.Errorf("uninstall previous: %w", err)
		}
	}

	if err := os.MkdirAll(s.installDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir install dir: %w", err)
	}
	if err := os.RemoveAll(target); err != nil {
		return nil, fmt.Errorf("clear target: %w", err)
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir target: %w", err)
	}

	// Write SKILL.md
	mdPath := filepath.Join(target, "SKILL.md")
	if err := os.WriteFile(mdPath, []byte(sk.SkillMD), 0o644); err != nil {
		return nil, fmt.Errorf("write SKILL.md: %w", err)
	}

	// Copy sibling files from the store's skill payload dir (if any)
	srcDir := s.SkillDir(sk.ID)
	if _, err := os.Stat(srcDir); err == nil {
		if err := copyTree(srcDir, target); err != nil {
			return nil, fmt.Errorf("copy files: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat src: %w", err)
	}

	if _, err := s.db.Exec(`
		INSERT INTO installations (skill_id, target_path, installed_at)
		VALUES (?, ?, ?)
		ON CONFLICT(skill_id) DO UPDATE SET target_path=excluded.target_path, installed_at=excluded.installed_at
	`, skillID, target, now()); err != nil {
		return nil, fmt.Errorf("record installation: %w", err)
	}
	return &Installation{
		SkillID:     skillID,
		SkillName:   sk.Name,
		SourceName:  sk.SourceName,
		TargetPath:  target,
		InstalledAt: now(),
	}, nil
}

// Uninstall removes the skill's install directory and its installations row.
// Returns (installation, nil) if it was installed, (nil, nil) if not.
func (s *Store) Uninstall(skillID int64) (*Installation, error) {
	var target string
	var installedAt int64
	err := s.db.QueryRow(`SELECT target_path, installed_at FROM installations WHERE skill_id=?`, skillID).Scan(&target, &installedAt)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	if err := os.RemoveAll(target); err != nil {
		return nil, fmt.Errorf("remove target: %w", err)
	}
	if _, err := s.db.Exec(`DELETE FROM installations WHERE skill_id=?`, skillID); err != nil {
		return nil, err
	}
	return &Installation{SkillID: skillID, TargetPath: target, InstalledAt: installedAt}, nil
}

// ListInstallations returns all currently installed skills.
func (s *Store) ListInstallations() ([]Installation, error) {
	rows, err := s.db.Query(`
		SELECT i.skill_id, s.name, src.name, i.target_path, i.installed_at
		FROM installations i
		JOIN skills s ON s.id = i.skill_id
		JOIN sources src ON src.id = s.source_id
		ORDER BY s.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Installation
	for rows.Next() {
		var inst Installation
		if err := rows.Scan(&inst.SkillID, &inst.SkillName, &inst.SourceName, &inst.TargetPath, &inst.InstalledAt); err != nil {
			return nil, err
		}
		out = append(out, inst)
	}
	return out, rows.Err()
}

func copyTree(srcDir, dstDir string) error {
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(dstDir, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		return copyFile(path, dst)
	})
}
