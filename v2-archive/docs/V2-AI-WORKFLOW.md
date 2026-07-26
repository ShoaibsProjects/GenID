# AI Engineering Team — Workflow Guide

> **Don't use one AI. Build an AI engineering team.**

---

## Model Rankings for Enterprise IAM

| Model | Architecture | Coding | Refactoring | Planning | Rating |
|-------|-------------|--------|-------------|----------|--------|
| **GPT-5.5** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | 10/10 |
| **Qwen3-Coder** | ⭐⭐⭐⭐☆ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐☆ | 9.8/10 |
| **Claude Opus** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐☆ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | 9.7/10 |
| **Kimi K2** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐☆ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | 9.6/10 |
| **DeepSeek V3/R1** | ⭐⭐⭐⭐☆ | ⭐⭐⭐⭐☆ | ⭐⭐⭐⭐☆ | ⭐⭐⭐⭐☆ | 9.2/10 |

---

## The AI Team

### 🧠 GPT-5.5 — Principal Architect

**Role:** Design the system, challenge assumptions, plan the roadmap

**Use for:**
- Architecture decisions
- Tradeoff analysis
- Scalability planning
- Security reviews
- Hiring-level critiques
- "What would Netflix/Apple/Google do?"

**Example prompts:**
```
Act as a Distinguished Engineer at Google.
Design the world's best Identity Platform.
Don't write code. Only architecture.
```

```
Review this architecture.
What would Netflix improve?
What would Apple reject?
What would Target reject?
What would Google redesign?
```

---

### 💻 Qwen3-Coder — Senior Software Engineer

**Role:** Implement the architecture (70-80% of work)

**Use for:**
- Go, Java, TypeScript, React, Next.js
- Kubernetes, Docker, Terraform
- CI/CD pipelines
- API design
- Refactoring
- Large architectural changes
- Test generation
- Performance improvements

**Example prompts:**
```
Implement Phase 1.
Follow clean architecture.
Production-ready.
Modular.
Enterprise quality.
No shortcuts.
```

---

### 🔍 Kimi K2 — Staff Engineer / Reviewer

**Role:** Review everything, find inconsistencies

**Use for:**
- Finding architectural gaps
- Code review
- Documentation
- Simplifying complex systems
- Identifying missing pieces

---

### 🧪 DeepSeek V3/R1 — Algorithm Engineer

**Role:** Graph algorithms, optimization, policy engines

**Use for:**
- Graph algorithms (Neo4j)
- Authorization logic optimization
- Policy engine internals
- Performance improvements
- Distributed systems algorithms

---

### 🎨 Claude Opus — Design Reviewer

**Role:** Brutally honest design reviews

**Use for:**
- Architecture critique
- Security analysis
- Distributed systems review
- "Destroy the design" sessions

---

## The Workflow

### Step 1: Architecture (GPT-5.5 or Qwen3.8-Max)

```
Act as a Distinguished Engineer at Google.
Design the next generation IAM platform.
```

Don't write code. Only architecture.

---

### Step 2: Implementation (Qwen3-Coder)

```
Implement Phase 1.
Follow clean architecture.
Production-ready.
Modular.
Enterprise quality.
No shortcuts.
```

---

### Step 3: Review (GPT-5.5 or Claude Opus)

```
Review this architecture.
What would Netflix improve?
What would Apple reject?
What would Target reject?
What would Google redesign?
```

---

### Step 4: Iterate (Qwen3-Coder)

Implement improvements. Repeat forever.

---

## The Pinned Prompt

```
You are no longer an AI assistant.

You are the Principal Engineer responsible for designing the successor to Okta.

You have previously designed systems at Google, Apple, Cloudflare, AWS, Stripe, Netflix, Microsoft Entra ID, and Auth0.

Your standards are extremely high.

Every piece of code must be production-grade.
Every API must scale to hundreds of millions of users.
Every architectural decision must consider:
- scalability
- security
- reliability
- observability
- maintainability
- developer experience
- distributed systems
- zero trust
- identity-first architecture

Never build a demo.
Build software that could realistically become a billion-dollar identity platform.

When reviewing my code, be brutally honest.
Reject anything that wouldn't pass a senior design review at Google or Apple.

Always explain tradeoffs before implementation.
```

---

## Usage Split

| Model | Usage | Purpose |
|-------|-------|---------|
| **Qwen3-Coder** | 70-80% | Implementation, refactoring, daily work |
| **GPT-5.5 / Qwen3.8-Max** | 20-30% | Architecture, design reviews, roadmap |

---

## The Hiring Committee Test

Use this prompt to evaluate your project:

```
Act as the hiring committee for:
Google, Apple, OpenAI, Anthropic, Cloudflare, Okta, Target, Microsoft, AWS, Netflix, Meta

Review this project exactly as if it were a hiring packet.

Score:
- Architecture
- Distributed Systems
- Security
- Identity
- Cloud
- Scalability
- Software Engineering
- Code Quality
- Originality
- Developer Experience
- Documentation
- Technical Depth
- Open Source Quality
- Interview Signal
- Hiring Signal

What would impress you?
What would disappoint you?
What would make this project unforgettable?
What would make you immediately schedule an interview?

Be brutally honest.
```

---

## The $5B Valuation Test

```
Pretend investors are about to value this company at $5 billion.

Audit the architecture.
What is missing that prevents this platform from becoming the next Okta?

Do not be nice.
Destroy the design.
Challenge every assumption.
Suggest technologies I have never considered.
Invent new IAM concepts.
Think 10 years ahead.

Assume AI agents become first-class identities.
Assume permissions become dynamic.
Assume passwords disappear.
Assume authorization happens continuously instead of login time.
Assume applications become AI-native.

How does IAM evolve?
Redesign accordingly.
```

---

**Last Updated:** 2026-07-22  
**Status:** Active — Use this workflow for V2 development
