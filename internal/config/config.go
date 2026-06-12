package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func intList(s string) []int {
	if s == "" {
		return nil
	}
	var out []int
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err == nil {
			out = append(out, n)
		}
	}
	return out
}

type Config struct {
	TelegramToken string
	OwnerTGID     int64
	RuvdsToken    string

	Datacenter         int
	AllowedDatacenters []int // если непусто — DC выбирается случайно из этого списка
	TariffID           int
	OSID               int
	PaymentPeriod      int
	CPU                int
	RAM                float64
	Drive              int
	DriveTariffID      int
	IPCount            int
	ComputerName       string
}

func Load() (*Config, error) {
	c := &Config{
		TelegramToken: os.Getenv("TELEGRAM_TOKEN"),
		RuvdsToken:    os.Getenv("RUVDS_TOKEN"),
		ComputerName:  envOr("COMPUTER_NAME", "auto-vds"),
	}

	if c.TelegramToken == "" {
		return nil, fmt.Errorf("TELEGRAM_TOKEN is required")
	}
	if c.RuvdsToken == "" {
		return nil, fmt.Errorf("RUVDS_TOKEN is required")
	}

	owner, err := strconv.ParseInt(os.Getenv("OWNER_TG_ID"), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("OWNER_TG_ID must be int64: %w", err)
	}
	c.OwnerTGID = owner

	c.Datacenter = mustInt("DEFAULT_DATACENTER", 1)
	c.AllowedDatacenters = intList(os.Getenv("ALLOWED_DATACENTERS"))
	c.TariffID = mustInt("DEFAULT_TARIFF_ID", 14)
	c.OSID = mustInt("DEFAULT_OS_ID", 52)
	c.PaymentPeriod = mustInt("DEFAULT_PAYMENT_PERIOD", 2)
	c.CPU = mustInt("DEFAULT_CPU", 1)
	c.Drive = mustInt("DEFAULT_DRIVE", 20)
	c.DriveTariffID = mustInt("DEFAULT_DRIVE_TARIFF_ID", 1)
	c.IPCount = mustInt("DEFAULT_IP_COUNT", 6)

	ram, err := strconv.ParseFloat(envOr("DEFAULT_RAM", "1"), 64)
	if err != nil {
		return nil, fmt.Errorf("DEFAULT_RAM: %w", err)
	}
	c.RAM = ram

	return c, nil
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func mustInt(k string, def int) int {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
