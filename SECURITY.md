# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 0.x     | :white_check_mark: |

## Reporting a Vulnerability

If you discover a security vulnerability, please report it responsibly:

1. **Do NOT** open a public GitHub issue
2. Email security@chaosplane.io with:
   - Description of the vulnerability
   - Steps to reproduce
   - Potential impact
3. You will receive an acknowledgment within 48 hours
4. We will work with you to understand and address the issue

## Security Measures

- All API endpoints require authentication
- Row-Level Security (RLS) enforces tenant isolation
- Agent communication uses mTLS
- Regular dependency scanning via Trivy
