package main

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	mcf "github.com/isucon/isucon14/webapp/go/mincostflow"
)

var mu sync.Mutex

// このAPIをインスタンス内から一定間隔で叩かせることで、椅子とライドをマッチングさせる
func internalGetMatching(w http.ResponseWriter, r *http.Request) {
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
	rides := []Ride{}
	if err := db.SelectContext(ctx, &rides, "SELECT * FROM rides WHERE chair_id IS NULL"); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	fmt.Printf("[DEBUG] rides count: %v\n", len(rides))
	if len(rides) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

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
	cLocs := []ChairLocation{}
	if err := db.SelectContext(ctx, &cLocs, `
		select cl.*
		from chairs c
		join chair_locations cl
		on  c.id = cl.chair_id
		and c.is_completed = 1
		and c.is_active = 1`); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	fmt.Printf("[DEBUG] chairs count: %v\n", len(cLocs))
	if len(cLocs) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	n := len(rides) + len(cLocs) + 2
	// 最小費用流
	mcf := mcf.NewMinCostFlow(n)

	// source -> chair
	for i, _ := range cLocs {
		mcf.AddEdge(0, i+1, 1, 0)
	}

	// chair -> ride
	for i, c := range cLocs {
		for j, r := range rides {
			distance := abs(c.Latitude-r.PickupLatitude) + abs(c.Longitude-r.PickupLongitude)
			mcf.AddEdge(i+1, len(cLocs)+j+1, 1, distance)
		}
	}

	// ride -> sink
	for j, _ := range rides {
		mcf.AddEdge(len(cLocs)+j+1, n-1, 1, 0)
	}

	// calc min path
	mcf.FlowL(0, n-1, mcf.Min(len(cLocs), len(rides)))

	// match
	for _, e := range mcf.Edges() {
		// 流量のあるEdgeだけを見る(source, sinkは除く)
		if e.Flow() == 0 || e.From() == 0 || e.To() == n-1 {
			continue
		}
		chairID := cLocs[e.From()-1].ChairID
		rideID := rides[e.To()-len(cLocs)-1].ID
		if err := match(ctx, chairID, rideID); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}

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
