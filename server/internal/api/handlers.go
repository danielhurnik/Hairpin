package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/danielhurnik/hairpin/server/internal/auth"
	"github.com/danielhurnik/hairpin/server/internal/game"
	"github.com/danielhurnik/hairpin/server/internal/store"
)

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "time": time.Now().UTC()})
}

// content serves the static game tables so the client can render names,
// prices and flavour without hardcoding a copy that can drift.
func (s *Server) content(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"chassis":    game.Chassis,
		"jobs":       game.Jobs,
		"items":      game.Items,
		"liveries":   game.Liveries,
		"tracks":     game.Tracks,
		"ruby_packs": game.RubyPacks,
		"slots":      game.Slots,
		"stat_names": game.DriverStatNames,
		"categories": game.UpgradeCategories,
		"max_tier":   game.MaxTier,
		"free_races": game.FreeRacesPerDay,
		"extra_cost": game.ExtraRaceRubies,
	})
}

// ---------------------------------------------------------------------------
// Auth
// ---------------------------------------------------------------------------

type deviceReq struct {
	DeviceID string `json:"device_id"`
}

// authDevice registers or resumes an anonymous device account.
//
// The device ID is a client-generated secret held in the OS keystore. We store
// only an HMAC of it. Anyone holding the secret is the account — which is the
// accepted trade-off for "no signup", and the reason this needs to grow into
// real account linking before launch.
func (s *Server) authDevice(w http.ResponseWriter, r *http.Request) {
	var req deviceReq
	if !decode(w, r, &req) {
		return
	}
	if len(req.DeviceID) < 16 || len(req.DeviceID) > 256 {
		fail(w, http.StatusBadRequest, "bad_device_id", "device_id must be 16-256 chars")
		return
	}
	hash := auth.HashDevice(req.DeviceID, s.cfg.TokenSecret)
	p, created, err := s.store.EnsurePlayer(r.Context(), hash)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token":   s.issuer.Issue(p.ID, time.Now()),
		"player":  p,
		"created": created,
	})
}

type nameReq struct {
	Name string `json:"name"`
}

func (s *Server) setName(w http.ResponseWriter, r *http.Request, p *store.Player) {
	var req nameReq
	if !decode(w, r, &req) {
		return
	}
	name := strings.TrimSpace(req.Name)
	if len(name) < 3 || len(name) > 20 {
		fail(w, http.StatusBadRequest, "bad_name", "name must be 3-20 characters")
		return
	}
	if err := s.store.SetDisplayName(r.Context(), p.ID, name); err != nil {
		writeErr(w, err)
		return
	}
	p2, err := s.store.GetPlayer(r.Context(), p.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p2)
}

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------

func (s *Server) me(w http.ResponseWriter, r *http.Request, p *store.Player) {
	build, items, err := s.store.BuildFor(r.Context(), p)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"player":      p,
		"build":       build,
		"rating":      game.OverallRating(build),
		"equipped":    items,
		"server_time": s.store.Now(),
	})
}

// state is the single call the client makes on launch: everything needed to
// render every screen, so the UI never has to guess at a value it should be
// asking the server for.
func (s *Server) state(w http.ResponseWriter, r *http.Request, p *store.Player) {
	ctx := r.Context()
	build, equipped, err := s.store.BuildFor(ctx, p)
	if err != nil {
		writeErr(w, err)
		return
	}
	job, err := s.store.ActiveJob(ctx, p.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	inv, err := s.store.Inventory(ctx, p.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	ladder, err := s.store.LadderAround(ctx, p, 21)
	if err != nil {
		writeErr(w, err)
		return
	}
	liveries, err := s.store.Liveries(ctx, p)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"player":      p,
		"build":       build,
		"rating":      game.OverallRating(build),
		"equipped":    equipped,
		"inventory":   inv,
		"active_job":  job,
		"job_board":   s.store.JobBoard(ctx, p),
		"garage":      s.store.GarageOptions(ctx, p, build),
		"ladder":      ladder,
		"liveries":    liveries,
		"server_time": s.store.Now(),
	})
}

// ---------------------------------------------------------------------------
// Jobs
// ---------------------------------------------------------------------------

func (s *Server) jobs(w http.ResponseWriter, r *http.Request, p *store.Player) {
	job, err := s.store.ActiveJob(r.Context(), p.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"board":       s.store.JobBoard(r.Context(), p),
		"active":      job,
		"server_time": s.store.Now(),
	})
}

type startJobReq struct {
	JobID string `json:"job_id"`
}

func (s *Server) startJob(w http.ResponseWriter, r *http.Request, p *store.Player) {
	var req startJobReq
	if !decode(w, r, &req) {
		return
	}
	job, err := s.store.StartJob(r.Context(), p.ID, req.JobID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"active": job, "server_time": s.store.Now()})
}

func (s *Server) claimJob(w http.ResponseWriter, r *http.Request, p *store.Player) {
	res, err := s.store.ClaimJob(r.Context(), p.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) rushJob(w http.ResponseWriter, r *http.Request, p *store.Player) {
	job, spent, err := s.store.RushJob(r.Context(), p.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"active": job, "rubies_spent": spent})
}

// ---------------------------------------------------------------------------
// Garage
// ---------------------------------------------------------------------------

func (s *Server) garage(w http.ResponseWriter, r *http.Request, p *store.Player) {
	build, equipped, err := s.store.BuildFor(r.Context(), p)
	if err != nil {
		writeErr(w, err)
		return
	}
	chassis, _ := game.ChassisByID(p.ChassisID)
	writeJSON(w, http.StatusOK, map[string]any{
		"player":   p,
		"chassis":  chassis,
		"build":    build,
		"rating":   game.OverallRating(build),
		"upgrades": s.store.GarageOptions(r.Context(), p, build),
		"equipped": equipped,
	})
}

type upgradeReq struct {
	Category string `json:"category"`
}

func (s *Server) buyUpgrade(w http.ResponseWriter, r *http.Request, p *store.Player) {
	var req upgradeReq
	if !decode(w, r, &req) {
		return
	}
	p2, paid, err := s.store.BuyUpgrade(r.Context(), p.ID, req.Category)
	if err != nil {
		writeErr(w, err)
		return
	}
	build, _, err := s.store.BuildFor(r.Context(), p2)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"player":   p2,
		"paid":     paid,
		"build":    build,
		"rating":   game.OverallRating(build),
		"upgrades": s.store.GarageOptions(r.Context(), p2, build),
	})
}

// ---------------------------------------------------------------------------
// Cars
// ---------------------------------------------------------------------------

type carReq struct {
	ChassisID string `json:"chassis_id"`
}

// carsPayload is the showroom: every car, what it costs, and what it would
// race at. Shared by all three car endpoints so a buy or a switch renders the
// updated list without a second round trip.
func (s *Server) carsPayload(w http.ResponseWriter, r *http.Request, p *store.Player, extra map[string]any) {
	build, _, err := s.store.BuildFor(r.Context(), p)
	if err != nil {
		writeErr(w, err)
		return
	}
	cars, err := s.store.Cars(r.Context(), p, build)
	if err != nil {
		writeErr(w, err)
		return
	}
	body := map[string]any{
		"player": p,
		"active": p.ChassisID,
		"build":  build,
		"rating": game.OverallRating(build),
		"cars":   cars,
	}
	for k, v := range extra {
		body[k] = v
	}
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) cars(w http.ResponseWriter, r *http.Request, p *store.Player) {
	s.carsPayload(w, r, p, nil)
}

func (s *Server) buyCar(w http.ResponseWriter, r *http.Request, p *store.Player) {
	var req carReq
	if !decode(w, r, &req) {
		return
	}
	p2, paid, err := s.store.BuyChassis(r.Context(), p.ID, req.ChassisID)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.carsPayload(w, r, p2, map[string]any{"paid": paid, "bought": req.ChassisID})
}

func (s *Server) equipCar(w http.ResponseWriter, r *http.Request, p *store.Player) {
	var req carReq
	if !decode(w, r, &req) {
		return
	}
	p2, err := s.store.SetActiveCar(r.Context(), p.ID, req.ChassisID)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.carsPayload(w, r, p2, nil)
}

type allocateReq struct {
	Stat   string `json:"stat"`
	Points int    `json:"points"`
}

func (s *Server) allocate(w http.ResponseWriter, r *http.Request, p *store.Player) {
	var req allocateReq
	if !decode(w, r, &req) {
		return
	}
	if req.Points == 0 {
		req.Points = 1
	}
	p2, err := s.store.AllocateStat(r.Context(), p.ID, req.Stat, req.Points)
	if err != nil {
		writeErr(w, err)
		return
	}
	build, _, err := s.store.BuildFor(r.Context(), p2)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"player": p2, "build": build, "rating": game.OverallRating(build)})
}

// ---------------------------------------------------------------------------
// Inventory
// ---------------------------------------------------------------------------

func (s *Server) inventory(w http.ResponseWriter, r *http.Request, p *store.Player) {
	inv, err := s.store.Inventory(r.Context(), p.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": inv, "slots": game.Slots})
}

type itemReq struct {
	ItemRowID int64 `json:"item_id"`
}

func (s *Server) equip(w http.ResponseWriter, r *http.Request, p *store.Player) {
	var req itemReq
	if !decode(w, r, &req) {
		return
	}
	if err := s.store.Equip(r.Context(), p.ID, req.ItemRowID); err != nil {
		writeErr(w, err)
		return
	}
	s.respondWithBuild(w, r, p.ID)
}

func (s *Server) unequip(w http.ResponseWriter, r *http.Request, p *store.Player) {
	var req itemReq
	if !decode(w, r, &req) {
		return
	}
	if err := s.store.Unequip(r.Context(), p.ID, req.ItemRowID); err != nil {
		writeErr(w, err)
		return
	}
	s.respondWithBuild(w, r, p.ID)
}

func (s *Server) sell(w http.ResponseWriter, r *http.Request, p *store.Player) {
	var req itemReq
	if !decode(w, r, &req) {
		return
	}
	got, err := s.store.Sell(r.Context(), p.ID, req.ItemRowID)
	if err != nil {
		writeErr(w, err)
		return
	}
	p2, err := s.store.GetPlayer(r.Context(), p.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	inv, err := s.store.Inventory(r.Context(), p.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cash_received": got, "player": p2, "items": inv})
}

func (s *Server) respondWithBuild(w http.ResponseWriter, r *http.Request, playerID string) {
	p, err := s.store.GetPlayer(r.Context(), playerID)
	if err != nil {
		writeErr(w, err)
		return
	}
	build, equipped, err := s.store.BuildFor(r.Context(), p)
	if err != nil {
		writeErr(w, err)
		return
	}
	inv, err := s.store.Inventory(r.Context(), playerID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"player": p, "build": build, "rating": game.OverallRating(build),
		"equipped": equipped, "items": inv,
	})
}

// ---------------------------------------------------------------------------
// Racing
// ---------------------------------------------------------------------------

func (s *Server) opponent(w http.ResponseWriter, r *http.Request, p *store.Player) {
	op, err := s.store.NextOpponent(r.Context(), p)
	if err != nil {
		writeErr(w, err)
		return
	}
	build, _, err := s.store.BuildFor(r.Context(), p)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"opponent":    op,
		"your_rating": game.OverallRating(build),
		"free_races":  p.FreeRaces,
		"extra_cost":  game.ExtraRaceRubies,
	})
}

func (s *Server) race(w http.ResponseWriter, r *http.Request, p *store.Player) {
	out, err := s.store.RunRace(r.Context(), p.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) history(w http.ResponseWriter, r *http.Request, p *store.Player) {
	h, err := s.store.RaceHistory(r.Context(), p.ID, intQuery(r, "limit", 20))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"races": h})
}

func (s *Server) replay(w http.ResponseWriter, r *http.Request, p *store.Player) {
	rep, err := s.store.Replay(r.Context(), p.ID, r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (s *Server) ladder(w http.ResponseWriter, r *http.Request, p *store.Player) {
	var entries []store.LadderEntry
	var err error
	if r.URL.Query().Get("around") == "me" {
		entries, err = s.store.LadderAround(r.Context(), p, intQuery(r, "limit", 21))
	} else {
		entries, err = s.store.Ladder(r.Context(), p.ID, intQuery(r, "from", 1), intQuery(r, "limit", 50))
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries, "your_rank": p.Rank})
}

// ---------------------------------------------------------------------------
// Shop
// ---------------------------------------------------------------------------

func (s *Server) shop(w http.ResponseWriter, r *http.Request, p *store.Player) {
	view, err := s.store.Shop(r.Context(), p)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

type buyOfferReq struct {
	OfferID string `json:"offer_id"`
}

func (s *Server) shopBuy(w http.ResponseWriter, r *http.Request, p *store.Player) {
	var req buyOfferReq
	if !decode(w, r, &req) {
		return
	}
	item, p2, err := s.store.BuyOffer(r.Context(), p.ID, req.OfferID)
	if err != nil {
		writeErr(w, err)
		return
	}
	view, err := s.store.Shop(r.Context(), p2)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": item, "player": p2, "shop": view})
}

func (s *Server) liveries(w http.ResponseWriter, r *http.Request, p *store.Player) {
	l, err := s.store.Liveries(r.Context(), p)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"liveries": l, "equipped": p.LiveryID})
}

type liveryReq struct {
	LiveryID string `json:"livery_id"`
}

func (s *Server) buyLivery(w http.ResponseWriter, r *http.Request, p *store.Player) {
	var req liveryReq
	if !decode(w, r, &req) {
		return
	}
	p2, err := s.store.BuyLivery(r.Context(), p.ID, req.LiveryID)
	if err != nil {
		writeErr(w, err)
		return
	}
	l, err := s.store.Liveries(r.Context(), p2)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"player": p2, "liveries": l})
}

func (s *Server) equipLivery(w http.ResponseWriter, r *http.Request, p *store.Player) {
	var req liveryReq
	if !decode(w, r, &req) {
		return
	}
	if err := s.store.EquipLivery(r.Context(), p.ID, req.LiveryID); err != nil {
		writeErr(w, err)
		return
	}
	p2, err := s.store.GetPlayer(r.Context(), p.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"player": p2})
}

type purchaseReq struct {
	PackID  string `json:"pack_id"`
	Receipt string `json:"receipt"`
}

// purchaseRubies credits a real-money purchase.
//
// MILESTONE 1 LIMITATION: with HAIRPIN_DEV_IAP=true any receipt string is
// accepted, which means this endpoint mints premium currency for free. Before
// this ships, `verifyReceipt` must call Apple's verifyReceipt / Google Play
// Developer API and confirm the product ID matches pack_id. The replay
// defence — a UNIQUE receipt column — is already in place.
func (s *Server) purchaseRubies(w http.ResponseWriter, r *http.Request, p *store.Player) {
	var req purchaseReq
	if !decode(w, r, &req) {
		return
	}
	if !s.cfg.DevIAP {
		fail(w, http.StatusNotImplemented, "iap_unverified",
			"store receipt validation is not implemented yet")
		return
	}
	if len(req.Receipt) < 8 {
		fail(w, http.StatusBadRequest, "bad_receipt", "receipt required")
		return
	}
	p2, granted, err := s.store.GrantRubies(r.Context(), p.ID, req.PackID, req.Receipt)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"player": p2, "rubies_granted": granted, "dev_mode": true})
}
