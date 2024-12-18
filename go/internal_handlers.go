package main

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	mcf "github.com/isucon/isucon14/webapp/go/mincostflow"
)

var mu sync.Mutex

type MatchPair struct {
	chairID string
	rideID  string
}

// このAPIをインスタンス内から一定間隔で叩かせることで、椅子とライドをマッチングさせる
func internalGetMatching(w http.ResponseWriter, r *http.Request) {
	st := time.Now()
	// 複数スレッドで同時実行されないように排他制御
	if !mu.TryLock() {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	defer mu.Unlock()
	ctx := r.Context()
	// MEMO: 一旦最も待たせているリクエストに適当な空いている椅子マッチさせる実装とする。おそらくもっといい方法があるはず…
	// ride := &Ride{}
	// if err := db.GetContext(ctx, ride, `SELECT * FROM rides WHERE chair_id IS NULL ORDER BY created_at LIMIT 1`); err != nil {
	// 	if errors.Is(err, sql.ErrNoRows) {
	// 		w.WriteHeader(http.StatusNoContent)
	// 		return
	// 	}
	// 	writeError(w, http.StatusInternalServerError, err)
	// 	return
	// }
	var chairs []Chair
	if err := db.SelectContext(ctx, &chairs, `
		select *
		from chairs
		where is_completed = 1
		and   is_active = 1`); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	fmt.Printf("[DEBUG1] chairs count: %v\n", len(chairs))
	if len(chairs) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	p1 := float32(time.Since(st).Milliseconds()) / 1000.0
	rides := []Ride{}
	if err := db.SelectContext(ctx, &rides, "SELECT * FROM rides WHERE chair_id IS NULL ORDER BY created_at LIMIT ?", 2*len(chairs)); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	fmt.Printf("[DEBUG1] rides count: %v\n", len(rides))
	if len(rides) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	p2 := float32(time.Since(st).Milliseconds()) / 1000.0
	// matched := &Chair{}
	// empty := false
	// for i := 0; i < 10; i++ {
	// 	if err := db.GetContext(ctx, matched, "SELECT * FROM chairs INNER JOIN (SELECT id FROM chairs WHERE is_active = TRUE ORDER BY RAND() LIMIT 1) AS tmp ON chairs.id = tmp.id LIMIT 1"); err != nil {
	// 		if errors.Is(err, sql.ErrNoRows) {
	// 			w.WriteHeader(http.StatusNoContent)
	// 			return
	// 		}
	// 		writeError(w, http.StatusInternalServerError, err)
	// 	}

	// 	if err := db.GetContext(ctx, &empty, "SELECT COUNT(*) = 0 FROM (SELECT COUNT(chair_sent_at) = 6 AS completed FROM ride_statuses WHERE ride_id IN (SELECT id FROM rides WHERE chair_id = ?) GROUP BY ride_id) is_completed WHERE completed = FALSE", matched.ID); err != nil {
	// 		writeError(w, http.StatusInternalServerError, err)
	// 		return
	// 	}
	// 	if empty {
	// 		break
	// 	}
	// }
	// if !empty {
	// 	w.WriteHeader(http.StatusNoContent)
	// 	return
	// }

	// var chairID string
	// if err := db.GetContext(ctx, &chairID, fmt.Sprintf(`
	// 	select c.id
	// 	from chairs c
	// 	join chair_locations cl
	// 	on  c.id = cl.chair_id
	// 	and c.is_completed = 1
	// 	and c.is_active = 1
	// 	order by (abs(cl.latitude - %d) + abs(cl.longitude - %d)) LIMIT 1`, ride.PickupLatitude, ride.PickupLongitude)); err != nil {
	// 	writeError(w, http.StatusInternalServerError, err)
	// 	return
	// }
	// TODO: chair_locationsの扱いが間違っているので、chairs TBLに最新の緯度経度をキャッシュして改善する

	n := len(rides) + len(chairs) + 2
	// 最小費用流
	mcf := mcf.NewMinCostFlow(n)

	// source -> chair
	for i, _ := range chairs {
		mcf.AddEdge(0, i+1, 1, 0)
	}

	// chair -> ride
	for i, c := range chairs {
		for j, r := range rides {
			cLoc := getLatestChairLoc(c.ID)
			distance := calculateDistance(cLoc.Latitude, cLoc.Longitude, r.PickupLatitude, r.PickupLongitude)
			speed := 1
			if s, ok := ChairSpeedbyName[c.Model]; ok {
				speed = s
			}
			time := distance / speed
			mcf.AddEdge(i+1, len(chairs)+j+1, 1, time)
		}
	}

	// ride -> sink
	for j, _ := range rides {
		mcf.AddEdge(len(chairs)+j+1, n-1, 1, 0)
	}
	p3 := float32(time.Since(st).Milliseconds()) / 1000.0
	// calc min path
	mcf.FlowL(0, n-1, mcf.Min(len(chairs), len(rides)))
	p4 := float32(time.Since(st).Milliseconds()) / 1000.0
	// match
	matchPair := []MatchPair{}
	for _, e := range mcf.Edges() {
		// 流量のあるEdgeだけを見る(source, sinkは除く)
		if e.Flow() == 0 || e.From() == 0 || e.To() == n-1 {
			continue
		}
		chairID := chairs[e.From()-1].ID
		rideID := rides[e.To()-len(chairs)-1].ID
		// if err := match(ctx, chairID, rideID); err != nil {
		// 	writeError(w, http.StatusInternalServerError, err)
		// 	return
		// }
		matchPair = append(matchPair, MatchPair{chairID, rideID})
	}
	var chairIDsComma, rideIDsComma string
	for i, mp := range matchPair {
		if i > 0 {
			chairIDsComma += ","
			rideIDsComma += ","
		}
		chairIDsComma += fmt.Sprintf("'%s'", mp.chairID)
		rideIDsComma += fmt.Sprintf("'%s'", mp.rideID)
		LatestRideCache.Delete(mp.chairID)
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf("UPDATE chairs SET is_completed = 0 WHERE id IN (%s)", chairIDsComma)); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	// UPDATE users SET address = ELT(FIELD(id, 2, 5, 6), '大阪', '愛知', '北海道') WHERE id IN (2, 5, 6);
	if _, err := db.ExecContext(ctx, fmt.Sprintf("UPDATE rides SET chair_id = ELT(FIELD(id, %s), %s) WHERE id IN (%s)", rideIDsComma, chairIDsComma, rideIDsComma)); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for _, e := range mcf.Edges() {
		// 流量のあるEdgeだけを見る(source, sinkは除く)
		if e.Flow() == 0 || e.From() == 0 || e.To() == n-1 {
			continue
		}
		chairID := chairs[e.From()-1].ID
		ride := rides[e.To()-len(chairs)-1]
		if _, ok := ChairNotifChan[chairID]; !ok {
			ChairNotifChan[chairID] = make(chan Notif, 5)
		}
		ChairNotifChan[chairID] <- Notif{
			Ride: &ride,
		}
		fmt.Printf("[DEBUG3] createRideStatus 03 ed: chairID: %v\n", chairID)
	}
	p5 := float32(time.Since(st).Milliseconds()) / 1000.0
	// 上で防いでいるはずなのに入れないと「椅子がライドの完了通知を受け取る前に、別の新しいライドの通知を受け取りました 」になるから追加
	// empty := false
	// if err := db.GetContext(ctx, &empty, "SELECT COUNT(*) = 0 FROM (SELECT COUNT(chair_sent_at) = 6 AS completed FROM ride_statuses WHERE ride_id IN (SELECT id FROM rides WHERE chair_id = ?) GROUP BY ride_id) is_completed WHERE completed = FALSE", chairID); err != nil {
	// 	writeError(w, http.StatusInternalServerError, err)
	// 	return
	// }
	// fmt.Printf("[DEBUG] empty: %v, chair_id: %s, ride.ID: %s\n", empty, chairID, ride.ID)
	// if !empty {
	// 	w.WriteHeader(http.StatusNoContent)
	// 	return
	// }
	// if _, err := db.ExecContext(ctx, "UPDATE chairs SET is_completed = 0 WHERE id = ?", chairID); err != nil {
	// 	writeError(w, http.StatusInternalServerError, err)
	// 	return
	// }
	// if _, err := db.ExecContext(ctx, "UPDATE rides SET chair_id = ? WHERE id = ?", chairID, ride.ID); err != nil {
	// 	writeError(w, http.StatusInternalServerError, err)
	// 	return
	// }
	// LatestRideCache.Delete(ride.ChairID)
	fmt.Printf("[DEBUG2] internalGetMatching: %.2f, %.2f, %.2f, %.2f, %.2f\n", p1, p2, p3, p4, p5)
	w.WriteHeader(http.StatusNoContent)
}

func match(ctx context.Context, chairID, rideID string) error {
	if _, err := db.ExecContext(ctx, "UPDATE chairs SET is_completed = 0 WHERE id = ?", chairID); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "UPDATE rides SET chair_id = ? WHERE id = ?", chairID, rideID); err != nil {
		return err
	}
	LatestRideCache.Delete(chairID)
	return nil
}
