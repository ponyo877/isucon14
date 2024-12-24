package main

import (
	crand "crypto/rand"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/go-json-experiment/json"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/kaz/pprotein/integration/standalone"
)

var db *sqlx.DB

func main() {
	go func() {
		standalone.Integrate(":19001")
	}()
	mux := setup()
	slog.Info("Listening on :8080")
	http.ListenAndServe(":8080", mux)
}

func setup() http.Handler {
	host := os.Getenv("ISUCON_DB_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	port := os.Getenv("ISUCON_DB_PORT")
	if port == "" {
		port = "3306"
	}
	_, err := strconv.Atoi(port)
	if err != nil {
		panic(fmt.Sprintf("failed to convert DB port number from ISUCON_DB_PORT environment variable into int: %v", err))
	}
	user := os.Getenv("ISUCON_DB_USER")
	if user == "" {
		user = "isucon"
	}
	password := os.Getenv("ISUCON_DB_PASSWORD")
	if password == "" {
		password = "isucon"
	}
	dbname := os.Getenv("ISUCON_DB_NAME")
	if dbname == "" {
		dbname = "isuride"
	}

	dbConfig := mysql.NewConfig()
	dbConfig.User = user
	dbConfig.Passwd = password
	dbConfig.Addr = net.JoinHostPort(host, port)
	dbConfig.Net = "tcp"
	dbConfig.DBName = dbname
	dbConfig.ParseTime = true
	dbConfig.InterpolateParams = true

	_db, err := sqlx.Connect("mysql", dbConfig.FormatDSN())
	if err != nil {
		panic(err)
	}
	db = _db
	mu = sync.Mutex{}
	db.SetMaxOpenConns(64)
	db.SetMaxIdleConns(64)

	mux := chi.NewRouter()
	mux.Use(middleware.Logger)
	mux.Use(middleware.Recoverer)
	mux.HandleFunc("POST /api/initialize", postInitialize)

	// app handlers
	{
		mux.HandleFunc("POST /api/app/users", appPostUsers)

		authedMux := mux.With(appAuthMiddleware)
		authedMux.HandleFunc("POST /api/app/payment-methods", appPostPaymentMethods)
		authedMux.HandleFunc("GET /api/app/rides", appGetRides)
		authedMux.HandleFunc("POST /api/app/rides", appPostRides)
		authedMux.HandleFunc("POST /api/app/rides/estimated-fare", appPostRidesEstimatedFare)
		authedMux.HandleFunc("POST /api/app/rides/{ride_id}/evaluation", appPostRideEvaluatation)
		authedMux.HandleFunc("GET /api/app/notification", appGetNotification)
		authedMux.HandleFunc("GET /api/app/nearby-chairs", appGetNearbyChairs)
	}

	// owner handlers
	{
		mux.HandleFunc("POST /api/owner/owners", ownerPostOwners)

		authedMux := mux.With(ownerAuthMiddleware)
		authedMux.HandleFunc("GET /api/owner/sales", ownerGetSales)
		authedMux.HandleFunc("GET /api/owner/chairs", ownerGetChairs)
	}

	// chair handlers
	{
		mux.HandleFunc("POST /api/chair/chairs", chairPostChairs)

		authedMux := mux.With(chairAuthMiddleware)
		authedMux.HandleFunc("POST /api/chair/activity", chairPostActivity)
		authedMux.HandleFunc("POST /api/chair/coordinate", chairPostCoordinate)
		authedMux.HandleFunc("GET /api/chair/notification", chairGetNotification)
		authedMux.HandleFunc("POST /api/chair/rides/{ride_id}/status", chairPostRideStatus)
	}

	// internal handlers
	{
		mux.HandleFunc("GET /api/internal/matching", internalGetMatching)
	}

	return mux
}

type postInitializeRequest struct {
	PaymentServer string `json:"payment_server"`
}

type postInitializeResponse struct {
	Language string `json:"language"`
}

func postInitialize(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	req := &postInitializeRequest{}
	if err := bindJSON(r, req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if out, err := exec.Command("../sql/init.sh").CombinedOutput(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to initialize: %s: %w", string(out), err))
		return
	}

	if _, err := db.ExecContext(ctx, "UPDATE settings SET value = ? WHERE name = 'payment_gateway_url'", req.PaymentServer); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	go func() {
		if _, err := http.Get("http://192.168.0.14:9000/api/group/collect"); err != nil {
			log.Printf("failed to communicate with pprotein: %v", err)
		}
	}()
	initCache()

	chairLocations := []ChairLocation{}
	if err := db.SelectContext(ctx, &chairLocations, "SELECT * FROM chair_locations ORDER BY created_at"); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for _, cl := range chairLocations {
		lazyDo, _ := createChairLocation(cl.ID, cl.ChairID, cl.Latitude, cl.Longitude, cl.CreatedAt)
		lazyDo()
	}

	chairModels := []ChairModel{}
	if err := db.SelectContext(ctx, &chairModels, "SELECT * FROM chair_models"); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for _, cm := range chairModels {
		chairSpeedbyName.Store(cm.Name, cm.Speed)
	}

	rideStatuses := []RideStatus{}
	if err := db.SelectContext(ctx, &rideStatuses, `
		SELECT rs1.* FROM ride_statuses rs1
		INNER JOIN (
			SELECT ride_id, MAX(created_at) AS max_created_at
			FROM ride_statuses
			GROUP BY ride_id
		) rs2 ON rs1.ride_id = rs2.ride_id AND rs1.created_at = rs2.max_created_at`); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for _, rs := range rideStatuses {
		latestRideStatusCache.Store(rs.RideID, rs.Status)
	}
	rides := []Ride{}
	if err := db.SelectContext(ctx, &rides, `
		SELECT r1.* FROM rides r1
		INNER JOIN (
			SELECT chair_id, MAX(updated_at) AS max_updated_at
			FROM rides
			GROUP BY chair_id
		) r2 ON r1.chair_id = r2.chair_id AND r1.updated_at = r2.max_updated_at`); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for _, r := range rides {
		latestRideCache.Store(r.ChairID, r)
	}
	rides = []Ride{}
	if err := db.SelectContext(ctx, &rides, "SELECT * FROM rides ORDER BY updated_at"); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for _, r := range rides {
		chairSales := []ChairSale{}
		if salesAny, ok := chairSaleCache.Load(r.ChairID.String); ok {
			chairSales = salesAny.([]ChairSale)
		}
		chairSales = append(chairSales, ChairSale{
			Sale:      calculateSale(r),
			UpdatedAt: r.UpdatedAt,
		})
		chairSaleCache.Store(r.ChairID.String, chairSales)
	}
	users := []User{}
	if err := db.SelectContext(ctx, &users, "SELECT * FROM users"); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for _, u := range users {
		createAppAccessToken(u.AccessToken, u)
	}
	chairs := []Chair{}
	if err := db.SelectContext(ctx, &chairs, "SELECT * FROM chairs"); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for _, c := range chairs {
		createChairCache(c.ID, c)
		createChairAccessToken(c.AccessToken, c)
		createChairsOwnerIDCache(c.OwnerID, c)
	}
	owners := []Owner{}
	if err := db.SelectContext(ctx, &owners, "SELECT * FROM owners"); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for _, o := range owners {
		createOwnerAccessToken(o.AccessToken, o)
		createOwnerCache(o.ID, o)
		createOwnerChairRegisterTokenCache(o.ChairRegisterToken, o)
	}
	codes := []string{}
	if err := db.SelectContext(ctx, &codes, "SELECT code FROM coupons WHERE code like 'INV_%'"); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for _, c := range codes {
		code := strings.Replace(c, "INV_", "", 1)
		incInvCouponCountCache(code)
	}
	coupons := []Coupon{}
	if err := db.SelectContext(ctx, &coupons, "SELECT * FROM coupons WHERE used_by IS NULL ORDER BY created_at"); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for _, c := range coupons {
		if strings.HasPrefix(c.Code, "CP_") {
			addUnusedCoupon(c.UserID, 3000)
		} else if strings.HasPrefix(c.Code, "INV_") {
			addUnusedCoupon(c.UserID, 1500)
		} else {
			addUnusedCoupon(c.UserID, 1000)
		}
	}
	coupons = []Coupon{}
	if err := db.SelectContext(ctx, &coupons, "SELECT * FROM coupons WHERE used_by IS NOT NULL ORDER BY created_at"); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for _, c := range coupons {
		if strings.HasPrefix(c.Code, "CP_") {
			createRideDiscountCache(*c.UsedBy, 3000)
		} else if strings.HasPrefix(c.Code, "INV_") {
			createRideDiscountCache(*c.UsedBy, 1500)
		} else {
			createRideDiscountCache(*c.UsedBy, 1000)
		}
	}

	writeJSON(w, http.StatusOK, postInitializeResponse{Language: "go"})
}

type Coordinate struct {
	Latitude  int `json:"latitude"`
	Longitude int `json:"longitude"`
}

func bindJSON(r *http.Request, v interface{}) error {
	return json.UnmarshalRead(r.Body, v)
}

func writeJSON(w http.ResponseWriter, statusCode int, v interface{}) {
	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	buf, err := json.Marshal(v)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(statusCode)
	w.Write(buf)
}

func writeError(w http.ResponseWriter, statusCode int, err error) {
	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	w.WriteHeader(statusCode)
	buf, marshalError := json.Marshal(map[string]string{"message": err.Error()})
	if marshalError != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"marshaling error failed"}`))
		return
	}
	w.Write(buf)

	slog.Error("error response wrote", err)
}

func secureRandomStr(b int) string {
	k := make([]byte, b)
	if _, err := crand.Read(k); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x", k)
}
