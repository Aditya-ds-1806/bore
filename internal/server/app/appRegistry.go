package app

import (
	"fmt"
	"sync"
)

type AppRegistry struct {
	apps  map[string]*App
	mutex *sync.RWMutex
}

func addApp(app *App) error {
	Registry.mutex.Lock()
	defer Registry.mutex.Unlock()

	if _, ok := Registry.apps[app.AppId]; ok {
		return fmt.Errorf("app with id %s already exists", app.AppId)
	}

	Registry.apps[app.AppId] = app
	return nil
}

func GetApp(appId string) (*App, bool) {
	Registry.mutex.RLock()
	defer Registry.mutex.RUnlock()
	app, ok := Registry.apps[appId]

	return app, ok
}

func DeleteApp(appId string) {
	Registry.mutex.Lock()
	defer Registry.mutex.Unlock()
	delete(Registry.apps, appId)
}

var Registry = &AppRegistry{
	apps:  make(map[string]*App),
	mutex: &sync.RWMutex{},
}
