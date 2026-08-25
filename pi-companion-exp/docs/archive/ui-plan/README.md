# Pi Companion UI plan

> Historical plan. The app now uses real REST and realtime networking. These files describe the initial mock-UI phase and are not current operating instructions.

Goal: document the initial mock/example UI work completed before server networking was added.

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
cd pi-companion-exp
.\gradlew.bat :app:assembleDebug
```

APK copy target:

```text
<artifact-directory>\pi-companion-debug.apk
```

Recommended new bottom nav:

```text
Home | Sessions | Workers | Settings
```
