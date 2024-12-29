package main

import (
	"sync"
)

var (
	chairAccessTokenCache = sync.Map{}
	appAccessTokenCache   = sync.Map{}
	appNotifChan          = sync.Map{}
	chairNotifChan        = sync.Map{}
)

func initCache() {
	chairAccessTokenCache = sync.Map{}
	appAccessTokenCache = sync.Map{}
	appNotifChan = sync.Map{}
	chairNotifChan = sync.Map{}
}

func getChairAccessToken(token string) (Chair, bool) {
	chair, ok := chairAccessTokenCache.Load(token)
	return chair.(Chair), ok
}

func createChairAccessToken(token string, chair Chair) {
	chairAccessTokenCache.Store(token, chair)
}

func getAppAccessToken(token string) (User, bool) {
	user, ok := appAccessTokenCache.Load(token)
	return user.(User), ok
}

func createAppAccessToken(token string, user User) {
	appAccessTokenCache.Store(token, user)
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
