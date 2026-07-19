# Aurora Bank

> A production-inspired digital banking platform that simulates enterprise banking architecture, end-to-end payment processing, and modern Quality Engineering practices.

Aurora Bank is an end-to-end banking simulation designed to mirror how modern financial institutions build, integrate, and test their systems. The project combines backend engineering, distributed system architecture, and enterprise test automation into a single platform.

The goal is to provide a realistic environment for developing and validating banking services, from customer onboarding and account management to interbank transfers using ISO 8583.

---

## System Architecture

```text
                           Customer
                               │
                ┌──────────────┴──────────────┐
                │                             │
        Internet Banking             Mobile Banking
                │                             │
                └──────────────┬──────────────┘
                               │
                         API Gateway
                               │
                     Transaction Hub (ESB)
                               │
                        Payment Switch
                    ┌──────────┴──────────┐
                    │                     │
           Internal Transfer      External Transfer
                    │                     │
              Core Banking        ISO 8583 Engine
                    │                     │
                    └──────────┬──────────┘
                               │
                          PostgreSQL
```

---

## Core Components

- Core Banking
- Customer Information File (CIF)
- Account Management
- Ledger Engine
- Journal Engine
- Transaction Engine
- API Gateway
- Transaction Hub (Enterprise Service Bus)
- Payment Switch
- ISO 8583 Simulator
- Internet Banking Channel
- Mobile Banking Channel

---

## Quality Engineering

- API Automation
- UI Automation (Playwright)
- Mobile Automation (Appium)
- Performance Testing
- Integration Testing
- Contract Testing
- Test Data Management

---

## Technology Stack

| Layer | Technology |
|--------|------------|
| Backend | Go |
| Database | PostgreSQL |
| API | REST |
| Container | Docker |
| Automation | Playwright, Appium |
| Version Control | Git |
| CI/CD | GitHub Actions |
| Messaging *(Planned)* | RabbitMQ |
| Monitoring *(Planned)* | Grafana, Prometheus |

---

## Project Vision

Aurora Bank is not intended to be another CRUD banking application.

It is designed as a production-inspired banking platform where engineers can explore:

- Enterprise banking architecture
- Distributed transaction flows
- ISO 8583 payment messaging
- Core banking concepts
- End-to-end Quality Engineering
- Modern backend development in Go

## Roadmap

### Phase 1 — Core Banking
- [ ] Customer (CIF)
- [ ] Account Management
- [ ] Ledger
- [ ] Journal
- [ ] Transaction Engine

### Phase 2 — Digital Channels
- [ ] Internet Banking
- [ ] Mobile Banking
- [ ] Authentication
- [ ] Beneficiary Management

### Phase 3 — Enterprise Integration
- [ ] API Gateway
- [ ] Transaction Hub (ESB)
- [ ] Payment Switch
- [ ] ISO 8583 Simulator

### Phase 4 — Quality Engineering
- [ ] API Automation
- [ ] UI Automation
- [ ] Mobile Automation
- [ ] Performance Testing
- [ ] CI/CD Pipeline