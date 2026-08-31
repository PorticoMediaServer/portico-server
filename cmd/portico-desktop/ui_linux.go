//go:build linux

package main

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
)

const (
	statusNotifierInterface = "org.kde.StatusNotifierItem"
	dbusMenuInterface       = "com.canonical.dbusmenu"
)

type linuxTray struct {
	app      *application
	conn     *dbus.Conn
	props    *prop.Properties
	menuProp *prop.Properties

	mu       sync.Mutex
	revision uint32
	items    map[int32]linuxMenuItem
}

type linuxMenuItem struct {
	label     string
	enabled   bool
	separator bool
	action    func()
}

type dbusMenuLayout struct {
	ID         int32
	Properties map[string]dbus.Variant
	Children   []dbus.Variant
}

type notifierToolTip struct {
	IconName string
	Pixmap   []struct {
		Width  int32
		Height int32
		Data   []byte
	}
	Title string
	Text  string
}

func runDesktopUI(ctx context.Context, app *application) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Portico companion: desktop status area is unavailable:", err)
		<-ctx.Done()
		return
	}
	defer conn.Close()
	name := fmt.Sprintf("org.kde.StatusNotifierItem-%d-1", os.Getpid())
	if reply, err := conn.RequestName(name, dbus.NameFlagDoNotQueue); err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		fmt.Fprintln(os.Stderr, "Portico companion: unable to register desktop status item")
		return
	}

	tray := &linuxTray{app: app, conn: conn, revision: 1, items: map[int32]linuxMenuItem{}}
	tray.rebuild(app.snapshot())
	if err := tray.export(); err != nil {
		fmt.Fprintln(os.Stderr, "Portico companion: unable to export desktop status item:", err)
		return
	}
	watcher := conn.Object("org.kde.StatusNotifierWatcher", dbus.ObjectPath("/StatusNotifierWatcher"))
	if call := watcher.Call("org.kde.StatusNotifierWatcher.RegisterStatusNotifierItem", 0, name); call.Err != nil {
		fmt.Fprintln(os.Stderr, "Portico companion: no compatible Linux status area was found:", call.Err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case status := <-app.updates:
			tray.rebuild(status)
			tray.mu.Lock()
			tray.revision++
			revision := tray.revision
			tray.mu.Unlock()
			_ = conn.Emit(dbus.ObjectPath("/Menu"), dbusMenuInterface+".LayoutUpdated", revision, int32(0))
			_ = tray.props.Set(statusNotifierInterface, "ToolTip", dbus.MakeVariant(notifierToolTip{IconName: "portico-media-server", Title: "Portico Media Server", Text: "Server: " + serverStatusLabel(status.Server) + " · Remote Access: " + status.RemoteLabel}))
		}
	}
}

func (t *linuxTray) export() error {
	statusSpec := map[string]map[string]*prop.Prop{
		statusNotifierInterface: {
			"Category":   {Value: "SystemServices", Emit: prop.EmitConst},
			"Id":         {Value: "portico-media-server", Emit: prop.EmitConst},
			"Title":      {Value: "Portico Media Server", Emit: prop.EmitConst},
			"Status":     {Value: "Active", Emit: prop.EmitTrue},
			"IconName":   {Value: "portico-media-server", Emit: prop.EmitConst},
			"ToolTip":    {Value: notifierToolTip{IconName: "portico-media-server", Title: "Portico Media Server", Text: "Server status"}, Emit: prop.EmitTrue},
			"ItemIsMenu": {Value: true, Emit: prop.EmitConst},
			"Menu":       {Value: dbus.ObjectPath("/Menu"), Emit: prop.EmitConst},
		},
	}
	props, err := prop.Export(t.conn, dbus.ObjectPath("/StatusNotifierItem"), statusSpec)
	if err != nil {
		return err
	}
	t.props = props
	if err := t.conn.Export(t, dbus.ObjectPath("/StatusNotifierItem"), statusNotifierInterface); err != nil {
		return err
	}
	menuSpec := map[string]map[string]*prop.Prop{
		dbusMenuInterface: {
			"Version":       {Value: uint32(3), Emit: prop.EmitConst},
			"TextDirection": {Value: "ltr", Emit: prop.EmitConst},
			"Status":        {Value: "normal", Emit: prop.EmitConst},
			"IconThemePath": {Value: []string{"/usr/share/icons/hicolor"}, Emit: prop.EmitConst},
		},
	}
	menuProps, err := prop.Export(t.conn, dbus.ObjectPath("/Menu"), menuSpec)
	if err != nil {
		return err
	}
	t.menuProp = menuProps
	return t.conn.Export(t, dbus.ObjectPath("/Menu"), dbusMenuInterface)
}

func (t *linuxTray) rebuild(status desktopStatus) {
	serverRunning := status.Server == "running"
	t.mu.Lock()
	defer t.mu.Unlock()
	t.items = map[int32]linuxMenuItem{
		1: {label: "Portico Media Server", enabled: false},
		2: {label: "Server: " + serverStatusLabel(status.Server), enabled: false},
		3: {label: "Remote Access: " + status.RemoteLabel, enabled: false},
		4: {separator: true},
		5: {label: "Open Portico Server", enabled: serverRunning, action: func() { _ = openBrowser(t.app.localURL) }},
		6: {label: "Open Portico Web", enabled: true, action: func() { _ = openBrowser(t.app.hostedURL) }},
		7: {label: "Server Settings", enabled: serverRunning, action: func() { _ = openBrowser(t.app.localURL + "/settings/server/status?tab=identity") }},
		8: {separator: true},
		9: {label: "Restart Server", enabled: serverRunning, action: func() { t.app.runServiceAction(serviceRestart) }},
		10: {label: map[bool]string{true: "Stop Server", false: "Start Server"}[serverRunning], enabled: true, action: func() {
			if serverRunning {
				t.app.runServiceAction(serviceStop)
			} else {
				t.app.runServiceAction(serviceStart)
			}
		}},
		11: {separator: true},
		12: {label: "Quit Portico Menu", enabled: true, action: func() { os.Exit(0) }},
	}
}

func (t *linuxTray) Activate(_ int32, _ int32) *dbus.Error          { return nil }
func (t *linuxTray) SecondaryActivate(_ int32, _ int32) *dbus.Error { return nil }
func (t *linuxTray) ContextMenu(_ int32, _ int32) *dbus.Error       { return nil }
func (t *linuxTray) Scroll(_ int32, _ string) *dbus.Error           { return nil }

func (t *linuxTray) GetLayout(parentID int32, _ int32, _ []string) (uint32, dbusMenuLayout, *dbus.Error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	children := make([]dbus.Variant, 0, len(t.items))
	for id := int32(1); id <= int32(len(t.items)); id++ {
		item := t.items[id]
		properties := map[string]dbus.Variant{"visible": dbus.MakeVariant(true)}
		if item.separator {
			properties["type"] = dbus.MakeVariant("separator")
		} else {
			properties["label"] = dbus.MakeVariant(item.label)
			properties["enabled"] = dbus.MakeVariant(item.enabled)
		}
		children = append(children, dbus.MakeVariant(dbusMenuLayout{ID: id, Properties: properties, Children: []dbus.Variant{}}))
	}
	return t.revision, dbusMenuLayout{ID: parentID, Properties: map[string]dbus.Variant{}, Children: children}, nil
}

func (t *linuxTray) Event(id int32, eventID string, _ dbus.Variant, _ uint32) *dbus.Error {
	if eventID != "clicked" {
		return nil
	}
	t.mu.Lock()
	item, ok := t.items[id]
	t.mu.Unlock()
	if ok && item.enabled && item.action != nil {
		go item.action()
	}
	return nil
}

func (t *linuxTray) EventGroup(events []struct {
	ID        int32
	EventID   string
	Data      dbus.Variant
	Timestamp uint32
}) []struct {
	ID    int32
	Error string
} {
	for _, event := range events {
		_ = t.Event(event.ID, event.EventID, event.Data, event.Timestamp)
	}
	return nil
}

func (t *linuxTray) AboutToShow(_ int32) (bool, *dbus.Error) { return false, nil }

func (t *linuxTray) AboutToShowGroup(ids []int32) ([]int32, []int32, *dbus.Error) {
	return nil, ids, nil
}

func (t *linuxTray) GetGroupProperties(ids []int32, _ []string) ([]struct {
	ID         int32
	Properties map[string]dbus.Variant
}, *dbus.Error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	result := make([]struct {
		ID         int32
		Properties map[string]dbus.Variant
	}, 0, len(ids))
	for _, id := range ids {
		item, ok := t.items[id]
		if !ok {
			continue
		}
		properties := map[string]dbus.Variant{"visible": dbus.MakeVariant(true)}
		if item.separator {
			properties["type"] = dbus.MakeVariant("separator")
		} else {
			properties["label"] = dbus.MakeVariant(item.label)
			properties["enabled"] = dbus.MakeVariant(item.enabled)
		}
		result = append(result, struct {
			ID         int32
			Properties map[string]dbus.Variant
		}{ID: id, Properties: properties})
	}
	return result, nil
}
