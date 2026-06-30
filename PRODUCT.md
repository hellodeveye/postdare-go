# Product

## Register

product

## Users

Postdare Go is for individual developers and small engineering teams who deploy services to one or a few Linux servers without Docker or Kubernetes. They are usually checking release status, triggering a deploy, reading logs, or recovering from a failed release under time pressure.

## Product Purpose

Postdare Go provides a lightweight CI/CD release console for bare-metal deployments. It connects GitHub and Gitee webhooks, runs a fixed test/build/deploy/health-check pipeline, records release history, streams deploy and application logs, sends failure notifications, supports rollback, and exposes safe MCP tools for AI-assisted release operations.

## Brand Personality

Calm, precise, operator-focused. The product should feel like a quiet control room: restrained, readable, fast to scan, and confident without becoming heavy.

## Anti-references

Avoid Jenkins-style clutter, old enterprise admin templates, glossy SaaS marketing dashboards, excessive gradients, oversized hero layouts, and dense RBAC-heavy control panels. The interface should not look like a generic back-office system or a toy pipeline builder.

## Design Principles

1. Put operational state first: every screen should make deploy health, current risk, and next action easy to see.
2. Prefer lists and timelines over decoration: releases, stages, logs, and webhook events are the primary material.
3. Keep high-risk actions explicit: deploy and rollback controls need clear labels, status feedback, and disabled/error states.
4. Make failure legible: failed stages, log excerpts, and suggested next steps should be close together.
5. Stay lightweight: first-version workflows should feel direct, not like a large CI platform compressed into a smaller window.

## Accessibility & Inclusion

Target WCAG AA contrast for text and controls. Support dark and light themes, visible focus states, keyboard navigation, reduced-motion preferences, and status colors that are paired with text labels rather than color alone.
