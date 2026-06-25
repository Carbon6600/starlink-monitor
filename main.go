package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image/color"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/clarkzjw/starlink-grpc-golang/pkg/spacex.com/api/device"
)

// --- Constants & Types ---

const (
	AppVersion     = "v1.0.1"
	RepoPath       = "Carbon6600/starlink-monitor"
	DefaultIP      = "192.168.100.1:9200"
	PollInterval   = 3 * time.Second
	RequestTimeout = 2 * time.Second
	WindowWidth    = 600
	WindowHeight   = 500
)

type GitHubRelease struct {
	TagName string `json:"tag_name"`
}

type DeviceState struct {
	IP              string
	Status          string
	GPSMode         string
	FirmwareVersion string
	UpdateAvailable bool
	RebootPending   bool
	Uptime          uint64
	AutoDisableGPS  bool
	LastUpdate      time.Time
	mu              sync.Mutex
	client          pb.DeviceClient
	conn            *grpc.ClientConn
}

type AppState struct {
	Devices      map[string]*DeviceState
	mu           sync.Mutex
	LogBox       *widget.Entry
	ListBox      *fyne.Container
	VersionLabel *widget.Label
}

var state = &AppState{
	Devices: make(map[string]*DeviceState),
}

// --- Helper Functions ---

func addLog(message string) {
	state.mu.Lock()
	defer state.mu.Unlock()
	timestamp := time.Now().Format("15:04:05")
	logEntry := fmt.Sprintf("[%s] %s\n", timestamp, message)
	state.LogBox.SetText(logEntry + state.LogBox.Text)
	// Keep only last 100 lines to prevent memory bloat
	lines := strings.Split(state.LogBox.Text, "\n")
	if len(lines) > 100 {
		state.LogBox.SetText(strings.Join(lines[:100], "\n"))
	}
}

func formatUptime(s uint64) string {
	h := s / 3600
	m := (s % 3600) / 60
	sec := s % 60
	return fmt.Sprintf("%dh %dm %ds", h, m, sec)
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Run()
}

func checkGitHubUpdate(win fyne.Window) {
	client := &http.Client{Timeout: 5 * time.Second}
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", RepoPath)
	resp, err := client.Get(url)
	if err != nil {
		return // Silent failure for update check
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return
	}

	if release.TagName != AppVersion {
		state.mu.Lock()
		if state.VersionLabel != nil {
			state.VersionLabel.SetText(fmt.Sprintf("Версія: %s (Доступне оновлення: %s)", AppVersion, release.TagName))
		}
		state.mu.Unlock()
		addLog(fmt.Sprintf("Нова версія програми доступна: %s", release.TagName))

		dialog.ShowConfirm("New Version Available",
			fmt.Sprintf("A new version %s is available. Would you like to download it?", release.TagName),
			func(confirmed bool) {
				if confirmed {
					releaseURL := fmt.Sprintf("https://github.com/%s/releases/latest", RepoPath)
					openBrowser(releaseURL)
				}
			}, win)
	}
}

func (ds *DeviceState) connect() error {
	ctx, cancel := context.WithTimeout(context.Background(), RequestTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, ds.IP,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock())
	if err != nil {
		return err
	}
	ds.conn = conn
	ds.client = pb.NewDeviceClient(conn)
	return nil
}

func (ds *DeviceState) disconnect() {
	if ds.conn != nil {
		ds.conn.Close()
	}
}

func (ds *DeviceState) disableGPS() error {
	ctx, cancel := context.WithTimeout(context.Background(), RequestTimeout)
	defer cancel()

	req := &pb.Request{
		Request: &pb.Request_DishSetConfig{
			DishSetConfig: &pb.DishSetConfigRequest{
				DishConfig: &pb.DishConfig{
					LocationRequestMode:      pb.DishConfig_NONE,
					ApplyLocationRequestMode: true,
				},
			},
		},
	}
	_, err := ds.client.Handle(ctx, req)
	return err
}

// --- Polling Logic ---

func pollDevice(ds *DeviceState) {
	for {
		if ds.conn == nil {
			if err := ds.connect(); err != nil {
				ds.mu.Lock()
				ds.Status = "Offline"
				ds.mu.Unlock()
				addLog(fmt.Sprintf("%s: Connection failed: %v", ds.IP, err))
				time.Sleep(PollInterval)
				continue
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), RequestTimeout)
		req := &pb.Request{
			Request: &pb.Request_GetStatus{
				GetStatus: &pb.GetStatusRequest{},
			},
		}
		resp, err := ds.client.Handle(ctx, req)
		cancel()

		if err != nil {
			ds.mu.Lock()
			ds.Status = "Offline"
			ds.mu.Unlock()
			addLog(fmt.Sprintf("%s: Request failed: %v", ds.IP, err))
			ds.disconnect()
			ds.conn = nil
		} else {
			var dishStatus *pb.DishGetStatusResponse
			if res, ok := resp.GetResponse().(*pb.Response_DishGetStatus); ok {
				dishStatus = res.DishGetStatus
			} else {
				addLog(fmt.Sprintf("%s: Unexpected response type", ds.IP))
				time.Sleep(PollInterval)
				continue
			}
			info := dishStatus.GetDeviceInfo()

			ds.mu.Lock()
			oldUptime := ds.Uptime

			ds.Status = "Up: " + formatUptime(dishStatus.GetDeviceState().GetUptimeS())
			ds.GPSMode = dishStatus.GetConfig().GetLocationRequestMode().String()
			ds.FirmwareVersion = info.GetSoftwareVersion()
			ds.UpdateAvailable = dishStatus.GetSoftwareUpdateState() != pb.SoftwareUpdateState_SOFTWARE_UPDATE_STATE_UNKNOWN
			ds.RebootPending = dishStatus.GetSwupdateRebootReady()
			ds.Uptime = dishStatus.GetDeviceState().GetUptimeS()
			ds.LastUpdate = time.Now()

			// Anti-Jamming Logic
			shouldDisable := false
			reason := ""

			if ds.AutoDisableGPS {
				// Check for reboot (Uptime reset or change)
				if oldUptime != 0 && ds.Uptime < oldUptime {
					shouldDisable = true
					reason = "Reboot detected"
				} else if strings.Contains(ds.GPSMode, "AUTO") {
					shouldDisable = true
					reason = "GPS mode switched to AUTO"
				}
			}
			ds.mu.Unlock()

			if shouldDisable {
				if err := ds.disableGPS(); err != nil {
					addLog(fmt.Sprintf("%s: Auto-disable GPS failed: %v", ds.IP, err))
				} else {
					addLog(fmt.Sprintf("%s: Auto-disabled GPS (%s)", ds.IP, reason))
				}
			}
		}
		time.Sleep(PollInterval)
	}
}

// --- GUI Components ---

func createDeviceRow(ds *DeviceState) fyne.CanvasObject {
	// IP Address
	ipLabel := widget.NewLabel(ds.IP)
	ipLabel.Wrapping = fyne.TextWrapWord

	// Status
	statusLabel := widget.NewLabel("Updating...")

	// GPS Mode
	gpsLabel := widget.NewLabel("Updating...")

	// Firmware/Update
	fwText := canvas.NewText("Updating...", color.White)

	// Auto-Disable Checkbox
	autoGPSCheck := widget.NewCheck("Auto-GPS Off", func(checked bool) {
		ds.mu.Lock()
		ds.AutoDisableGPS = checked
		ds.mu.Unlock()
	})
	autoGPSCheck.Checked = true // Default active

	// Manual Disable Button
	disableBtn := widget.NewButton("OFF GPS", func() {
		if err := ds.disableGPS(); err != nil {
			addLog(fmt.Sprintf("%s: Manual disable failed: %v", ds.IP, err))
		} else {
			addLog(fmt.Sprintf("%s: GPS disabled manually", ds.IP))
		}
	})

	// Delete Button
	deleteBtn := widget.NewButton("X", func() {
		state.mu.Lock()
		delete(state.Devices, ds.IP)
		state.mu.Unlock()
		ds.disconnect()
		refreshDeviceList()
	})

	row := container.NewGridWithColumns(6,
		ipLabel, statusLabel, gpsLabel, container.NewMax(fwText), autoGPSCheck,
		container.NewHBox(disableBtn, deleteBtn),
	)

	// Update labels in a loop
	go func() {
		for {
			ds.mu.Lock()
			statusLabel.SetText(ds.Status)
			gpsLabel.SetText(ds.GPSMode)

			fwString := fmt.Sprintf("%s", ds.FirmwareVersion)
			if ds.UpdateAvailable {
				fwText.Color = color.RGBA{R: 0, G: 255, B: 0, A: 255} // Green
				fwString += " [Оновлення вночі]"
			} else {
				fwText.Color = color.White
			}
			if ds.RebootPending {
				fwString += " [REB!]"
			}
			fwText.Text = fwString
			fwText.Refresh()
			ds.mu.Unlock()
			time.Sleep(1 * time.Second)
		}
	}()

	return row
}

func refreshDeviceList() {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.ListBox.Objects = nil

	// Header
	header := container.NewGridWithColumns(6,
		widget.NewLabel("IP Address"),
		widget.NewLabel("Status"),
		widget.NewLabel("GPS Mode"),
		widget.NewLabel("Firmware"),
		widget.NewLabel("Auto-Off"),
		widget.NewLabel("Actions"),
	)
	state.ListBox.Add(header)

	for _, ds := range state.Devices {
		state.ListBox.Add(createDeviceRow(ds))
	}
	state.ListBox.Refresh()
}

func addDevice(ip string) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return
	}
	if !strings.Contains(ip, ":") {
		ip += ":9200"
	}

	state.mu.Lock()
	if _, exists := state.Devices[ip]; exists {
		state.mu.Unlock()
		return
	}
	ds := &DeviceState{
		IP:             ip,
		AutoDisableGPS: true,
	}
	state.Devices[ip] = ds
	state.mu.Unlock()

	go pollDevice(ds)
	refreshDeviceList()
	addLog(fmt.Sprintf("%s: Added to monitor", ip))
}

func main() {
	myApp := app.New()

	iconData, err := os.ReadFile("app.ico")
	if err == nil {
		myApp.SetIcon(fyne.NewStaticResource("app.ico", iconData))
	}

	win := myApp.NewWindow("Starlink Micro-Dashboard")
	win.Resize(fyne.NewSize(WindowWidth, WindowHeight))

	// --- Top Section: Add Device ---
	ipEntry := widget.NewEntry()
	ipEntry.SetPlaceHolder("IP:Port (e.g. 192.168.100.1:9200)")
	ipEntry.SetText(DefaultIP)

	addBtn := widget.NewButton("Add", func() {
		addDevice(ipEntry.Text)
	})

	topBar := container.NewBorder(nil, nil, nil, addBtn, ipEntry)

	// --- Middle Section: Device List ---
	state.ListBox = container.NewVBox()
	scrollList := container.NewVScroll(state.ListBox)

	// --- Bottom Section: Logs ---
	state.LogBox = widget.NewMultiLineEntry()
	logHeader := widget.NewLabel("Mini-Logs:")
	state.VersionLabel = widget.NewLabel("Версія: " + AppVersion)
	state.VersionLabel.Alignment = fyne.TextAlignTrailing
	logContainer := container.NewVBox(logHeader, container.NewStack(state.LogBox), state.VersionLabel)

	// Main Layout
	content := container.NewBorder(topBar, logContainer, nil, nil, scrollList)
	win.SetContent(content)

	// Check for updates in a separate goroutine
	go checkGitHubUpdate(win)

	// Initial Device
	ipEntry.SetText(DefaultIP)
	addDevice(DefaultIP)

	win.ShowAndRun()
}
