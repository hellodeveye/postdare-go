# Design

## Mood

Late-night release room: quiet screens, crisp status lights, and enough contrast to read logs without strain.

## Color Strategy

Restrained product UI. The surface is near-black in dark mode and pure white in light mode. A deep rose/plum primary anchors selection and high-risk actions; teal, amber, red, and blue carry semantic states.

## Palette

```css
:root {
  --background: oklch(0.995 0 0);
  --surface: oklch(0.970 0.003 340);
  --surface-2: oklch(0.935 0.006 340);
  --border: oklch(0.865 0.008 340);
  --ink: oklch(0.180 0.018 340);
  --muted: oklch(0.440 0.020 340);
  --primary: oklch(0.410 0.145 340);
  --primary-ink: oklch(0.990 0 0);
  --accent: oklch(0.620 0.135 185);
  --success: oklch(0.570 0.130 155);
  --warning: oklch(0.700 0.140 75);
  --danger: oklch(0.590 0.170 25);
  --info: oklch(0.610 0.130 245);
}

.dark {
  --background: oklch(0.085 0 0);
  --surface: oklch(0.125 0.004 340);
  --surface-2: oklch(0.175 0.007 340);
  --border: oklch(0.255 0.008 340);
  --ink: oklch(0.925 0.006 340);
  --muted: oklch(0.650 0.010 340);
  --primary: oklch(0.620 0.150 340);
  --primary-ink: oklch(0.990 0 0);
  --accent: oklch(0.700 0.135 185);
  --success: oklch(0.700 0.135 155);
  --warning: oklch(0.780 0.135 75);
  --danger: oklch(0.690 0.170 25);
  --info: oklch(0.720 0.120 245);
}
```

## Typography

Use system UI fonts for all product surfaces. Keep a tight product scale with 12px labels, 14px body, 16px section titles, 20px page titles, and 13px monospace logs.

## Components

Buttons use 8px radius, strong focus rings, and consistent heights. Cards are reserved for metrics, repeated project rows, and detail panels. Tables and timeline rows carry most operational data. Status badges always include text labels.

## Motion

Use 150-200ms transitions for hover, focus, and active states. Respect reduced-motion preferences and avoid page-load choreography.
