package service

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"modul/internal/config"
	"modul/internal/model"
	"modul/internal/probe"
	"modul/internal/ruvds"
)

// Service связывает RuVDS API, TCP-probe и репозиторий БД.
type Service struct {
	cfg   *config.Config
	repo  *Repository
	ruvds *ruvds.Client

	dcCache    map[int]string
	dcCacheExp time.Time
	dcMu       sync.Mutex
}

func New(cfg *config.Config, repo *Repository, client *ruvds.Client) *Service {
	return &Service{cfg: cfg, repo: repo, ruvds: client}
}

// DatacenterName возвращает человеко-читаемое имя ДЦ по id,
// кэшируя список с RuVDS на час. Если не удалось — возвращает "DC <id>".
func (s *Service) DatacenterName(ctx context.Context, id int) string {
	s.dcMu.Lock()
	defer s.dcMu.Unlock()

	if time.Now().After(s.dcCacheExp) || s.dcCache == nil {
		dcs, err := s.ruvds.ListDatacenters(ctx)
		if err == nil {
			s.dcCache = make(map[int]string, len(dcs))
			for _, d := range dcs {
				s.dcCache[d.ID] = d.Name
			}
			s.dcCacheExp = time.Now().Add(time.Hour)
		}
	}
	if name, ok := s.dcCache[id]; ok {
		return name
	}
	return fmt.Sprintf("DC %d", id)
}

// ListHistory возвращает всю историю серверов (для команды "Логи").
func (s *Service) ListHistory(limit int) ([]model.Server, error) {
	return s.repo.ListAll(limit)
}

// CreateResult — то, что хочет показать бот после /create.
type CreateResult struct {
	Server     *model.Server
	CostRub    float64
	Datacenter int
}

// Create создаёт сервер на RuVDS, ждёт готовности, забирает IP,
// пингует их и сохраняет всё в БД.
func (s *Service) Create(ctx context.Context) (*CreateResult, error) {
	dc := s.pickDatacenter()
	name := fmt.Sprintf("%s-%d", s.cfg.ComputerName, time.Now().Unix())

	resp, err := s.ruvds.CreateServer(ctx, ruvds.ServerCreateReq{
		Datacenter:    dc,
		TariffID:      s.cfg.TariffID,
		OSID:          s.cfg.OSID,
		PaymentPeriod: s.cfg.PaymentPeriod,
		CPU:           s.cfg.CPU,
		RAM:           s.cfg.RAM,
		Drive:         s.cfg.Drive,
		DriveTariffID: s.cfg.DriveTariffID,
		IP:            s.cfg.IPCount,
		ComputerName:  name,
		UserComment:   "created via tg-bot",
	})
	if err != nil {
		return nil, fmt.Errorf("create server: %w", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	act, err := s.ruvds.WaitAction(waitCtx, resp.Action.ID, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("wait action: %w", err)
	}
	if act.Status != "success" {
		return nil, fmt.Errorf("action finished with status %q", act.Status)
	}

	nets, err := s.ruvds.GetNetworks(ctx, resp.VirtualServerID)
	if err != nil {
		return nil, fmt.Errorf("get networks: %w", err)
	}

	addrs := make([]string, 0, len(nets.V4))
	for _, n := range nets.V4 {
		addrs = append(addrs, n.IPAddress)
	}
	results := probe.CheckAll(addrs, 3*time.Second)

	srv := &model.Server{
		VirtualServerID: resp.VirtualServerID,
		Datacenter:      dc,
		Password:        resp.Password,
		ComputerName:    name,
		IPs:             make([]model.IP, 0, len(results)),
	}
	for _, r := range results {
		srv.IPs = append(srv.IPs, model.IP{
			Address: r.IP,
			Alive:   r.Alive,
			Port:    r.Port,
		})
	}

	if err := s.repo.Save(srv); err != nil {
		// сервер на RuVDS создан, в БД не сохранён — это критично логировать,
		// но возвращаем результат, чтобы пользователь хотя бы увидел IP.
		return &CreateResult{Server: srv, CostRub: resp.CostRub, Datacenter: dc},
			fmt.Errorf("db save (server created on ruvds anyway): %w", err)
	}

	return &CreateResult{Server: srv, CostRub: resp.CostRub, Datacenter: dc}, nil
}

// Delete снимает сервер на RuVDS и помечает запись в БД.
func (s *Service) Delete(ctx context.Context, virtualServerID int) error {
	if _, err := s.ruvds.DeleteServer(ctx, virtualServerID); err != nil {
		return fmt.Errorf("ruvds delete: %w", err)
	}
	if err := s.repo.MarkDeleted(virtualServerID); err != nil {
		return fmt.Errorf("repo mark deleted: %w", err)
	}
	return nil
}

// ListActive возвращает активные серверы (для кнопки «Инфо»).
func (s *Service) ListActive() ([]model.Server, error) {
	return s.repo.ListActive()
}

// GetByVirtualServerID возвращает сервер по id из RuVDS вместе с его IP.
func (s *Service) GetByVirtualServerID(vsID int) (*model.Server, error) {
	return s.repo.GetByVirtualServerID(vsID)
}

func (s *Service) pickDatacenter() int {
	if len(s.cfg.AllowedDatacenters) == 0 {
		return s.cfg.Datacenter
	}
	return s.cfg.AllowedDatacenters[rand.Intn(len(s.cfg.AllowedDatacenters))]
}
