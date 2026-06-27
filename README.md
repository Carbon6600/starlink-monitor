# Starlink Monitor

A lightweight, compact dashboard for monitoring Starlink dishes in real-time via gRPC.

<img width="448" height="391" alt="image" src="https://github.com/user-attachments/assets/bc4d6145-4495-4700-bfe4-9d441f568088" />





## 🚀 Features

- **Multi-Device Monitoring**: Track multiple Starlink dishes from a single interface.
- **Real-time Stats**: Monitor uptime, GPS mode, and firmware version.
- **Anti-Jamming / Auto-Disable**: Hardware-level GPS inhibition. Automatically disables GPS based on real-time status checks to prevent location tracking.
- **Compact GUI**: Built with Fyne for a minimal and efficient user experience.
- **Quick Management**: Easily add or remove dishes from the monitor.

## 🛠️ Installation

### Prerequisites
- [Go](https://go.dev/dl/) (latest version recommended)
- C compiler (gcc) for Fyne dependencies on Windows (e.g., Mingw-w64)

### Setup
1. Clone the repository:
   ```bash
   git clone https://github.com/Carbon6600/starlink-monitor.git
   cd starlink-monitor
   ```
2. Install dependencies:
   ```bash
   go mod tidy
   ```
3. Build the application:
   ```bash
   go build -ldflags="-s -w -H=windowsgui" -o starlink_security_v1.0.0.exe main.go
   ```
   Alternatively, run the provided `build.bat` on Windows.

## 📖 Usage

1. Run the application:
   ```bash
   ./starlink-monitor.exe
   ```
2. Enter the IP address and port of your Starlink dish (e.g., `192.168.100.1:9200`).
3. Click **Add** to start monitoring.
4. Use the **Auto-GPS Off** checkbox to enable the automatic GPS disabling logic.
5. Click **OFF GPS** to manually disable GPS on the selected dish.

## 📦 Dependencies
- [Fyne v2](https://fyne.io/) - GUI Toolkit
- [gRPC-Go](https://grpc.io/) - Communication with Starlink API
- [starlink-grpc-golang](https://github.com/clarkzjw/starlink-grpc-golang) - Starlink API Protobuf definitions

## ⚖️ License
This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
