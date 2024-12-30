package main

import (
	"sync"
)

var (
	chairAccessToken = sync.Map{}
	appAccessToken   = sync.Map{}
	appNotifChan     = sync.Map{}
	chairNotifChan   = sync.Map{}
)

func initCache() {
	chairAccessToken = sync.Map{}
	appAccessToken = sync.Map{}
	appNotifChan = sync.Map{}
	chairNotifChan = sync.Map{}
	chairSpeedbyName = map[string]int{}
}

func getChairAccessToken(token string) (Chair, bool) {
	chair, ok := chairAccessToken.Load(token)
	return chair.(Chair), ok
}

func createChairAccessToken(token string, chair Chair) {
	chairAccessToken.Store(token, chair)
}

func getAppAccessToken(token string) (User, bool) {
	user, ok := appAccessToken.Load(token)
	return user.(User), ok
}

func createAppAccessToken(token string, user User) {
	appAccessToken.Store(token, user)
}

func getAppChan(userID string) chan *appGetNotificationResponseData {
	appChan, ok := appNotifChan.Load(userID)
	if !ok {
		appNotifChan.Store(userID, make(chan *appGetNotificationResponseData, 5))
		appChan, _ = appNotifChan.Load(userID)
	}
	return appChan.(chan *appGetNotificationResponseData)
}

func getChairChan(chairID string) chan *chairGetNotificationResponseData {
	chairChan, ok := chairNotifChan.Load(chairID)
	if !ok {
		chairNotifChan.Store(chairID, make(chan *chairGetNotificationResponseData, 5))
		chairChan, _ = chairNotifChan.Load(chairID)
	}
	return chairChan.(chan *chairGetNotificationResponseData)
}

func getChairSpeedbyName(name string) int {
	return chairSpeedbyName[name]
}

var chairSpeedbyName = map[string]int{
	"AeroSeat":        3,
	"Aurora Glow":     7,
	"BalancePro":      3,
	"ComfortBasic":    2,
	"EasySit":         2,
	"ErgoFlex":        3,
	"Infinity Seat":   5,
	"Legacy Chair":    7,
	"LiteLine":        2,
	"LuxeThrone":      5,
	"Phoenix Ultra":   7,
	"ShadowEdition":   7,
	"SitEase":         2,
	"StyleSit":        3,
	"Titanium Line":   5,
	"ZenComfort":      5,
	"アルティマシート X":      5,
	"インフィニティ GEAR V":  7,
	"インペリアルクラフト LUXE": 5,
	"ヴァーチェア SUPREME":  7,
	"エアシェル ライト":       2,
	"エアフロー EZ":        3,
	"エコシート リジェネレイト":   7,
	"エルゴクレスト II":      3,
	"オブシディアン PRIME":   7,
	"クエストチェア Lite":    3,
	"ゲーミングシート NEXUS":  3,
	"シェルシート ハイブリッド":   3,
	"シャドウバースト M":      5,
	"ステルスシート ROGUE":   5,
	"ストリームギア S1":      3,
	"スピンフレーム 01":      2,
	"スリムライン GX":       5,
	"ゼノバース ALPHA":     7,
	"ゼンバランス EX":       5,
	"タイタンフレーム ULTRA":  7,
	"チェアエース S":        2,
	"ナイトシート ブラックエディション": 7,
	"フォームライン RX":        3,
	"フューチャーステップ VISION": 7,
	"フューチャーチェア CORE":    5,
	"プレイスタイル Z":         3,
	"フレックスコンフォート PRO":   3,
	"プレミアムエアチェア ZETA":   5,
	"プロゲーマーエッジ X1":      5,
	"ベーシックスツール プラス":     2,
	"モーションチェア RISE":     5,
	"リカーブチェア スマート":      3,
	"リラックスシート NEO":      2,
	"リラックス座":            2,
	"ルミナスエアクラウン":        7,
	"匠座 PRO LIMITED":    7,
	"匠座（たくみざ）プレミアム":     7,
	"雅楽座":        5,
	"風雅（ふうが）チェア": 3,
}
