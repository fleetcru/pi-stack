# Pi Companion UI Design Guidelines

These notes document the small spacing/layout fixes made during the mock UI pass so future screens and components do not repeat the same problems.

## Overall visual direction

Pi Companion uses a dark, grayscale, card-based interface.

Preferred style:
- dark background
- rounded cards
- soft borders
- white primary text
- muted grey secondary text
- restrained accent color only for active/selected/send states
- generous spacing between sections

Avoid:
- content touching screen edges
- page titles starting at absolute x=0
- floating bottom nav touching the left/right edges
- dense settings rows without breathing room
- default text fields that look too boxy for chat input

---

## Page margins

Every main page should have horizontal content padding.

Recommended page padding:

```kotlin
Modifier.padding(horizontal = 18.dp)
```

For page titles, add a small extra start offset so the title is not flush with the card edges or screen edge:

```kotlin
Column(
  Modifier.padding(start = 4.dp, top = 28.dp, bottom = 16.dp)
) {
  // title + subtitle
}
```

Use this pattern for pages like:
- Sessions
- Settings
- Workers
- future detail/index pages

---

## Page header spacing

Recommended page header:

```kotlin
Column(Modifier.padding(start = 4.dp, top = 28.dp, bottom = 16.dp)) {
  Text(
    text = "Page title",
    style = MaterialTheme.typography.headlineMedium,
    fontWeight = FontWeight.Bold,
  )
  Text(
    text = "Short page subtitle",
    style = MaterialTheme.typography.bodyMedium,
    color = MaterialTheme.colorScheme.onSurfaceVariant,
  )
}
```

Do not place page headers at the extreme top-left of the screen.

---

## Card spacing

Cards should have enough vertical space between them.

Recommended list spacing:

```kotlin
verticalArrangement = Arrangement.spacedBy(14.dp)
```

Recommended section spacing on vertical pages:

```kotlin
verticalArrangement = Arrangement.spacedBy(14.dp)
```

Recommended card inner padding:

```kotlin
Modifier.padding(horizontal = 16.dp, vertical = 15.dp)
```

For settings-style cards, use slightly more vertical padding:

```kotlin
Modifier.padding(horizontal = 16.dp, vertical = 18.dp)
```

---

## Settings screens

Settings pages need extra breathing room because rows are dense.

Page container:

```kotlin
Column(
  modifier
    .fillMaxSize()
    .verticalScroll(rememberScrollState())
    .padding(horizontal = 18.dp),
  verticalArrangement = Arrangement.spacedBy(14.dp),
)
```

Settings section body:

```kotlin
Column(Modifier.padding(horizontal = 16.dp, vertical = 18.dp))
```

Settings row vertical padding:

```kotlin
Modifier.padding(vertical = 12.dp)
```

Toggle row vertical padding:

```kotlin
Modifier.padding(vertical = 9.dp)
```

Bottom spacer should clear the floating nav:

```kotlin
Column(Modifier.padding(bottom = 112.dp)) {}
```

---

## Sessions page

The Sessions page should feel like a mobile sidebar, but it still needs page margins.

Recommended container:

```kotlin
Column(
  modifier
    .fillMaxSize()
    .padding(horizontal = 18.dp),
)
```

Recommended session list:

```kotlin
LazyColumn(
  modifier = Modifier.padding(top = 14.dp),
  verticalArrangement = Arrangement.spacedBy(14.dp),
)
```

Recommended session card row padding:

```kotlin
Modifier.padding(horizontal = 16.dp, vertical = 15.dp)
```

Avoid cramped cards or list items touching the screen edge.

---

## Floating bottom navigation

The bottom nav should feel like a floating pill, not a full-width system bar.

Recommended placement:

```kotlin
BottomNavBar(
  modifier = Modifier
    .align(Alignment.BottomCenter)
    .padding(start = 20.dp, end = 20.dp, bottom = 24.dp),
)
```

Rules:
- always leave left/right screen margins
- keep it centered
- keep it above the bottom edge
- ensure scrollable pages have bottom padding so content is not hidden behind it

Recommended scrollable page bottom padding:

```kotlin
contentPadding = PaddingValues(bottom = 100.dp)
```

or for settings pages:

```kotlin
Column(Modifier.padding(bottom = 112.dp)) {}
```

---

## Chat composer

The chat composer should look like a rounded assistant-style composer, not a default form field.

Preferred structure:
- large rounded surface
- transparent text field inside
- round send button on the right
- no plus icon for now
- multiline input up to 4 lines

Recommended outer shape:

```kotlin
Surface(
  shape = RoundedCornerShape(24.dp),
  color = MaterialTheme.colorScheme.surfaceVariant,
  border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
)
```

Recommended send button:

```kotlin
Box(
  modifier = Modifier
    .size(42.dp)
    .clip(CircleShape)
    .background(if (canSend) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.surface),
  contentAlignment = Alignment.Center,
)
```

Use an upward arrow icon for send:

```kotlin
Icons.Rounded.ArrowUpward
```

Do not use a plus/attachment icon until attachments/tools are implemented.

---

## Detail page back behavior

For now, the session detail/chat back button should return the user to the Sessions page.

Expected behavior:
- tapping a session opens Session Detail
- tapping back in Session Detail returns to Sessions

This is more predictable than returning to Home when sessions were opened from different places.

---

## Component rule of thumb

When creating a new screen:

1. Add horizontal page padding first.
2. Offset the title slightly from the left.
3. Use 14.dp spacing between major sections/cards.
4. Use 16–18.dp inner card padding.
5. Add bottom padding if the floating nav is visible.
6. Avoid default Material components when they look too boxy; wrap/customize them to match the app style.
