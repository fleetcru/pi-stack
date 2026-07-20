# 10 Testing Checklist

Build:

```powershell
cd C:\Users\basin\Desktop\pi-companion
.\gradlew.bat assembleDebug
```

Copy APK:

```powershell
Copy-Item .\app\build\outputs\apk\debug\app-debug.apk C:\Users\basin\Desktop\pi-companion-apk\pi-companion-debug.apk -Force
```

Manual server test:

1. Start pi-server:

```powershell
cd C:\Users\basin\pi-server
$env:PI_SERVER_ALLOWED_ROOTS = "C:\Users\basin\pi-server,C:\Users\basin\Desktop"
go run .\cmd\pi-server --addr 0.0.0.0:3141
```

2. On Android settings, set server URL to Tailscale/private host, for example:

```text
http://100.x.x.x:3141
```

3. Test connection.
4. Open Sessions tab.
5. Open a session detail screen.
6. Confirm WebSocket connects.
7. Send a prompt.
8. Confirm server events appear.
9. Turn server off and verify offline errors are clean.

Automated tests later:
- repository tests with MockWebServer
- JSON parsing tests
- settings DataStore tests
- ViewModel state transition tests
