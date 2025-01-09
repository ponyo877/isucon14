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
	"time"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v2"
	"github.com/kaz/pprotein/integration/standalone"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/go-chi/chi/v5"
	"github.com/go-sql-driver/mysql"
	pb "github.com/isucon/isucon14/webapp/go/grpc"
	"github.com/jmoiron/sqlx"
)

var db *sqlx.DB
var paymentGatewayURL string
var client pb.SubServiceClient
var benchStartedAt time.Time

func main() {
	go func() {
		standalone.Integrate(":19001")
	}()
	mux := setup()
	muxNotification := setupNotification()
	go http.ListenAndServe(":8081", muxNotification)
	listenAddr := net.JoinHostPort("", strconv.Itoa(8080))
	if err := mux.Listen(listenAddr); err != nil {
		fmt.Printf("failed to listen: %v", err)
	}
}

func setupNotification() http.Handler {
	mux := chi.NewRouter()
	mux.With(appAuthMiddleware).HandleFunc("GET /api/app/notification", appGetNotification)
	mux.With(chairAuthMiddleware).HandleFunc("GET /api/chair/notification", chairGetNotification)
	return mux
}

func setup() *fiber.App {
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

	subAddress := "192.168.0.12:8081"
	conn, err := grpc.NewClient(subAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic(err)
	}
	// defer conn.Close()

	client = pb.NewSubServiceClient(conn)

	mux := fiber.New(fiber.Config{
		JSONEncoder:  sonic.Marshal,
		JSONDecoder:  sonic.Unmarshal,
		ErrorHandler: errorResponseHandler,
	})
	mux.Post("/api/initialize", postInitialize)
	mux.Post("/api/app/users", appPostUsers)
	mux.Post("/api/owner/owners", ownerPostOwners)
	mux.Post("/api/chair/chairs", chairPostChairs)

	// app handlers
	{
		authedMuxApp := mux.Group("/api/app")
		authedMuxApp.Use(appAuthMiddlewareFiber)
		authedMuxApp.Post("/payment-methods", appPostPaymentMethods)
		authedMuxApp.Get("/rides", appGetRides)
		authedMuxApp.Post("/rides", appPostRides)
		authedMuxApp.Post("/rides/estimated-fare", appPostRidesEstimatedFare)
		authedMuxApp.Post("/rides/:ride_id/evaluation", appPostRideEvaluatation)
		// authedMuxApp.Get("/notification", appGetNotification)
		authedMuxApp.Get("/nearby-chairs", appGetNearbyChairs)
	}

	// owner handlers
	{
		authedMuxOwner := mux.Group("/api/owner")
		authedMuxOwner.Use(ownerAuthMiddlewareFiber)
		authedMuxOwner.Get("/sales", ownerGetSales)
		authedMuxOwner.Get("/chairs", ownerGetChairs)
	}

	// chair handlers
	{
		authedMuxChair := mux.Group("/api/chair")
		authedMuxChair.Use(chairAuthMiddlewareFiber)
		authedMuxChair.Post("/activity", chairPostActivity)
		authedMuxChair.Post("/coordinate", chairPostCoordinate)
		// authedMuxChair.Get("/notification", chairGetNotification)
		authedMuxChair.Post("/rides/:ride_id/status", chairPostRideStatus)
	}

	return mux
}

type postInitializeRequest struct {
	PaymentServer string `json:"payment_server"`
}

type postInitializeResponse struct {
	Language string `json:"language"`
}

func postInitialize(c *fiber.Ctx) error {
	ctx := c.Context()
	req := &postInitializeRequest{}
	if err := c.BodyParser(req); err != nil {
		return c.SendStatus(fiber.StatusBadRequest)
	}

	if _, err := exec.Command("../sql/init.sh").CombinedOutput(); err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	if _, err := db.ExecContext(ctx, "UPDATE settings SET value = ? WHERE name = 'payment_gateway_url'", req.PaymentServer); err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	go func() {
		if _, err := http.Get("http://192.168.0.14:9000/api/group/collect"); err != nil {
			log.Printf("failed to communicate with pprotein: %v", err)
		}
	}()
	initCache()

	chairLocations := []ChairLocation{}
	if err := db.SelectContext(ctx, &chairLocations, "SELECT * FROM chair_locations ORDER BY created_at"); err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	for _, cl := range chairLocations {
		chairLocation := &ChairLocation{
			ID:        cl.ID,
			ChairID:   cl.ChairID,
			Latitude:  cl.Latitude,
			Longitude: cl.Longitude,
			CreatedAt: cl.CreatedAt,
		}
		before, ok := getLatestChairLocation(cl.ChairID)
		createChairLocation(cl.ChairID, chairLocation)
		if ok {
			distance := calculateDistance(before.Latitude, before.Longitude, cl.Latitude, cl.Longitude)
			createChairTotalDistance(cl.ChairID, distance, cl.CreatedAt)
		}
	}

	rideStatuses := []RideStatus{}
	if err := db.SelectContext(ctx, &rideStatuses, `
		SELECT rs1.* FROM ride_statuses rs1
		INNER JOIN (
			SELECT ride_id, MAX(created_at) AS max_created_at
			FROM ride_statuses
			GROUP BY ride_id
		) rs2 ON rs1.ride_id = rs2.ride_id AND rs1.created_at = rs2.max_created_at`); err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	for _, rs := range rideStatuses {
		createLatestRideStatus(rs.RideID, rs.Status)
	}
	rides := []Ride{}
	if err := db.SelectContext(ctx, &rides, `
		SELECT r1.* FROM rides r1
		INNER JOIN (
			SELECT chair_id, MAX(updated_at) AS max_updated_at
			FROM rides
			GROUP BY chair_id
		) r2 ON r1.chair_id = r2.chair_id AND r1.updated_at = r2.max_updated_at`); err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	for _, r := range rides {
		createLatestRide(r.ChairID.String, &r)
	}
	rides = []Ride{}
	if err := db.SelectContext(ctx, &rides, "SELECT * FROM rides ORDER BY updated_at"); err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	for _, r := range rides {
		createChairSale(&r)
	}
	users := []User{}
	if err := db.SelectContext(ctx, &users, "SELECT * FROM users"); err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	for _, u := range users {
		createAppAccessToken(u.AccessToken, &u)
	}
	chairs := []Chair{}
	if err := db.SelectContext(ctx, &chairs, "SELECT * FROM chairs"); err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	for _, c := range chairs {
		c.Speed = getChairSpeedbyName(c.Model)
		createChair(c.ID, &c)
		createChairAccessToken(c.AccessToken, &c)
		createChairsOwnerID(c.OwnerID, &c)
	}
	owners := []Owner{}
	if err := db.SelectContext(ctx, &owners, "SELECT * FROM owners"); err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	for _, o := range owners {
		createOwnerAccessToken(o.AccessToken, &o)
		createOwner(o.ID, &o)
		createOwnerChairRegisterToken(o.ChairRegisterToken, &o)
	}
	codes := []string{}
	if err := db.SelectContext(ctx, &codes, "SELECT code FROM coupons WHERE code like 'INV_%'"); err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	for _, c := range codes {
		code := strings.Replace(c, "INV_", "", 1)
		incInvCouponCount(code)
	}
	coupons := []Coupon{}
	if err := db.SelectContext(ctx, &coupons, "SELECT * FROM coupons WHERE used_by IS NULL ORDER BY created_at"); err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
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
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	for _, c := range coupons {
		if strings.HasPrefix(c.Code, "CP_") {
			createRideDiscount(*c.UsedBy, 3000)
		} else if strings.HasPrefix(c.Code, "INV_") {
			createRideDiscount(*c.UsedBy, 1500)
		} else {
			createRideDiscount(*c.UsedBy, 1000)
		}
	}
	users = []User{}
	if err := db.SelectContext(ctx, &users, "SELECT * FROM users"); err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	for _, u := range users {
		createUser(u.ID, &u)
		createUserInv(u.InvitationCode, &u)
	}
	rides = []Ride{}
	if err := db.SelectContext(ctx, &rides, "SELECT * FROM rides"); err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	for _, r := range rides {
		createRide(r.ID, &r)
	}
	if err := db.GetContext(ctx, &paymentGatewayURL, "SELECT value FROM settings WHERE name = 'payment_gateway_url'"); err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	rides = []Ride{}
	if err := db.SelectContext(ctx, &rides, "SELECT * FROM rides ORDER BY created_at"); err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	for _, r := range rides {
		addRideIDsUserID(r.UserID, r.ID)
	}
	benchStartedAt = time.Now()
	go startMatchingLoop()
	return c.JSON(postInitializeResponse{Language: "go"})
}

type Coordinate struct {
	Latitude  int `json:"latitude"`
	Longitude int `json:"longitude"`
}

func writeError(w http.ResponseWriter, statusCode int, err error) {
	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	w.WriteHeader(statusCode)

	err = sonic.ConfigFastest.NewEncoder(w).Encode(map[string]string{"message": err.Error()})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"marshaling error failed"}`))
		return
	}

	slog.Error("error response wrote", err)
}

type ErrorResponse struct {
	Error string `json:"message"`
}

func errorResponseHandler(c *fiber.Ctx, err error) error {
	fmt.Printf("error at %s: %+v", c.Path(), err)
	if he, ok := err.(*fiber.Error); ok {
		return c.Status(he.Code).JSON(&ErrorResponse{Error: err.Error()})
	}

	return c.Status(http.StatusInternalServerError).JSON(&ErrorResponse{Error: err.Error()})
}

func secureRandomStr(b int) string {
	k := make([]byte, b)
	if _, err := crand.Read(k); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x", k)
}
