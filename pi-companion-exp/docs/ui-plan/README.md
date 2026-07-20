# Pi Companion UI Plan

Goal: expand the Android app with mock/example UI screens before wiring real server networking.

Style requirements:
- Kotlin + Jetpack Compose
- black / white / grey theme
- rounded cards
- Material Icons Extended
- small focused files
- mock data only for now
- no real networking yet

Build command:

```powershell
cd C:\Users\basin\Desktop\pi-companion
.\gradlew.bat assembleDebug
```

APK copy target:

```text
C:\Users\basin\Desktop\pi-companion-apk\pi-companion-debug.apk
```

Recommended new bottom nav:

```text
Home | Sessions | Workers | Settings
```
