# 08 Acceptance Criteria

> Historical planning document. The current app code, tests, and server OpenAPI document are authoritative.

The implementation is done when:

1. App builds successfully:

```powershell
cd <project-root>\pi-companion-exp
.\gradlew.bat assembleDebug
```

2. APK is copied to:

```text
<artifact-directory>\pi-companion-debug.apk
```

3. Added mock pages:
- Sessions screen
- Session detail/chat screen
- Settings screen

4. Bottom nav is:

```text
Home | Sessions | Workers | Settings
```

5. No real networking added yet.

6. Files remain small and focused.

7. UI uses the existing dark grayscale style and Material icons.
