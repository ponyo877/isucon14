package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
)

// このAPIをインスタンス内から一定間隔で叩かせることで、椅子とライドをマッチングさせる
func internalGetMatching(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// MEMO: 一旦最も待たせているリクエストに適当な空いている椅子マッチさせる実装とする。おそらくもっといい方法があるはず…
	ride := &Ride{}
	if err := db.GetContext(ctx, ride, `SELECT * FROM rides WHERE chair_id IS NULL ORDER BY created_at LIMIT 1`); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	var cnt int
	if err := db.GetContext(ctx, &cnt, "SELECT COUNT(*) FROM rides WHERE chair_id IS NULL"); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	fmt.Printf("[DEBUG] waiting rides count: %d\n", cnt)

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
	var chairID string
	if err := db.GetContext(ctx, &chairID, fmt.Sprintf(`select
		    uc.cid
		from
		    (
		        select
		            ac.cid cid
		        from
		            ride_statuses rs
		            join
		                (
		                    select
		                        r.id rid,
		                        c.id cid
		                    from
		                        rides r
		                        join
		                            chairs c
		                        on  r.chair_id = c.id
		                    where
		                        c.is_active = TRUE
		                ) AS ac
		            on  rs.ride_id = ac.rid
		        group by
		            ac.cid
		        having (count(rs.chair_sent_at) %% 6 = 0)
				UNION ALL SELECT c.id AS chair_id FROM chairs c LEFT JOIN rides r ON c.id = r.chair_id WHERE r.chair_id IS NULL
		    ) AS uc
		    join
		        chair_locations cl
		    on  uc.cid = cl.chair_id
		order by
		    (abs(cl.latitude - %d) + abs(cl.longitude - %d))
		LIMIT 1`, ride.PickupLatitude, ride.PickupLongitude)); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	// 上で防いでいるはずなのに入れないと「椅子がライドの完了通知を受け取る前に、別の新しいライドの通知を受け取りました 」になるから追加
	empty := false
	if err := db.GetContext(ctx, &empty, "SELECT COUNT(*) = 0 FROM (SELECT COUNT(chair_sent_at) = 6 AS completed FROM ride_statuses WHERE ride_id IN (SELECT id FROM rides WHERE chair_id = ?) GROUP BY ride_id) is_completed WHERE completed = FALSE", chairID); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if !empty {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if _, err := db.ExecContext(ctx, "UPDATE rides SET chair_id = ? WHERE id = ?", chairID, ride.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	LatestRideCache.Delete(ride.ChairID)

	w.WriteHeader(http.StatusNoContent)
}
