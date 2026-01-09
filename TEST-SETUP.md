# Stellar Lab Test Setup Guide

## Overview

The project now includes comprehensive testing capabilities with automated CI/CD integration. Tests run automatically on all pull requests to the main branch and must pass before merging.

## Test Scripts

### test-cluster.sh
Enhanced testing script that runs various scenarios and validates expected behavior.

**Usage:**
```bash
# Run quick smoke tests (3 nodes, 1 minute)
./test-cluster.sh quick

# Run comprehensive test suite (5 nodes, 5 minutes)
./test-cluster.sh all

# Run with custom configuration
STELLAR_TEST_NODES=10 STELLAR_TEST_DURATION=600 ./test-cluster.sh all

# Keep test data after completion for debugging
./test-cluster.sh all --keep

# Clean up test artifacts
./test-cluster.sh clean
```

**Test Scenarios:**
1. **Cluster Formation** - Validates all nodes start successfully
2. **Peer Discovery** - Ensures nodes discover each other via DHT
3. **DHT Operations** - Verifies PING, FIND_NODE, ANNOUNCE messages
4. **Attestations** - Checks attestation creation and verification
5. **Coordinate Validation** - Validates coordinate generation and positioning
6. **Star System Generation** - Checks star class distribution and genesis black hole
7. **API Endpoints** - Tests all REST API endpoints
8. **Name Validation** - Ensures invalid names are rejected
9. **Network Resilience** - Tests node failure and rejoin scenarios

**Output:**
- JSON report: `.test-cluster/reports/test-report.json`
- Human-readable summary: `.test-cluster/reports/summary.txt`
- Node logs: `.test-cluster/logs/node*.log`

### dev-cluster.sh (Original)
Still available for quick manual development testing.

```bash
# Start 5-node cluster
./dev-cluster.sh start

# Check status
./dev-cluster.sh status

# View logs
./dev-cluster.sh logs 2  # View logs for node 2

# Stop cluster
./dev-cluster.sh stop
```

## GitHub Actions Integration

### Automatic PR Testing

The test suite runs automatically on:
- All pull requests to `main` branch (quick tests)
- Pushes to `main` branch (comprehensive tests)
- Manual workflow dispatch (configurable)

### Setting Up Branch Protection

To make tests required for merging:

1. Go to Settings → Branches in your GitHub repository
2. Add a branch protection rule for `main`
3. Enable "Require status checks to pass before merging"
4. Search for and select "Run Tests" as a required check
5. Enable "Require branches to be up to date before merging"
6. Save changes

Now all PRs must pass tests before they can be merged to main.

## Test Configuration

### Environment Variables

- `STELLAR_TEST_NODES`: Number of nodes to test (default: 5)
- `STELLAR_TEST_DURATION`: Test duration in seconds (default: 300)
- `VERBOSE`: Enable verbose output (0 or 1)

### Test Reports

Reports include:
- Test summary with pass/fail counts
- Individual test results with details
- Log parsing for errors and panics
- Message count metrics (PING, ANNOUNCE, etc.)
- Network topology information

## Local Testing Workflow

1. **Before committing changes:**
   ```bash
   # Run quick tests locally
   ./test-cluster.sh quick
   ```

2. **Before creating a PR:**
   ```bash
   # Run comprehensive tests
   ./test-cluster.sh all
   ```

3. **Debug test failures:**
   ```bash
   # Keep test data for investigation
   ./test-cluster.sh all --keep
   
   # Check logs
   cat .test-cluster/logs/node*.log | grep ERROR
   
   # View test report
   cat .test-cluster/reports/summary.txt
   ```

## CI/CD Workflow

1. Developer creates feature branch and makes changes
2. Developer runs tests locally
3. Developer creates PR to main
4. GitHub Actions automatically runs quick tests
5. Tests must pass before PR can be merged
6. After merge, comprehensive tests run on main branch

## Troubleshooting

### Tests fail locally but pass in CI
- Check Go version matches CI (1.21)
- Ensure `bc` is installed for calculations
- Clean test environment: `./test-cluster.sh clean`

### Port conflicts
- Tests use ports 9081-9085 (web) and 9871-9875 (DHT)
- Original dev cluster uses 8081-8085 and 7871-7875
- Ensure no conflicts with running services

### Timeout issues
- Increase timeout: `STELLAR_TEST_DURATION=600 ./test-cluster.sh all`
- Check system resources (CPU, memory)
- Reduce node count for faster tests

## Adding New Tests

To add new test scenarios, edit `test-cluster.sh`:

1. Create a test function:
   ```bash
   test_my_feature() {
       log_test "Testing my feature..."
       
       # Test logic here
       local result=$(some_test_command)
       
       validate_test "my_feature_test" "[[ condition ]]" "Details: $result"
   }
   ```

2. Add to `run_all_tests()` function:
   ```bash
   test_my_feature
   ```

3. Test locally before committing

## Future Improvements

- Performance benchmarking
- Load testing with many nodes
- Chaos testing (random failures)
- Integration tests with Docker
- Test coverage reporting