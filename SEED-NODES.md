# Seed Nodes Guide

## What Are Seed Nodes?

Seed nodes are well-known, publicly accessible nodes that help new members discover the Stellar Mesh network. They act as entry points for the network but do not control it.

## How It Works

The seed node list is maintained collaboratively on GitHub:

1. **Seed list lives at:** `SEED-NODES.txt` in the main repository
2. **Nodes fetch list at startup** from GitHub's raw content URL
3. **Community updates list** via pull requests
4. **No code updates needed** - changes take effect immediately for new nodes

### Discovery Flow

```
1. New node starts without -bootstrap flag
2. Fetches SEED-NODES.txt from GitHub
3. Connects to one of the listed seed nodes
4. Seed node shares its peer list (via exchangePeers protocol)
5. New node discovers other peers organically
6. New node can now operate independently
```

## Adding Your Seed Node

### Requirements

1. **Stable public IP or domain name**
   - Static IP address, or
   - Domain name (e.g., `myseed.example.com`)

2. **Publicly accessible**
   - Port 8080 (or your chosen port) open to the internet
   - Firewall configured to allow incoming connections

3. **Good uptime**
   - 95%+ uptime recommended
   - Seed nodes should be reliable

4. **Basic resources**
   - Any VPS/cloud instance works
   - 1 GB RAM, 1 CPU core is plenty
   - Minimal bandwidth usage

### Setup Instructions

```bash
# 1. Run stellar-mesh on a stable port
./stellar-mesh -name "Seed Node 1" -address "0.0.0.0:8080"

# 2. Ensure it's publicly accessible
# Test from another machine:
curl http://YOUR_IP:8080/api/system

# 3. Keep it running with good uptime (95%+)
```

### Submit Your Seed Node

**Option 1: GitHub Pull Request (Recommended)**

1. Fork the stellar-mesh repository
2. Edit `SEED-NODES.txt`
3. Add your address on a new line: `your-ip-or-domain:8080`
4. Submit a pull request with:
   - Your node's address
   - Expected uptime commitment
   - Optional: Contact info for downtime notifications

**Example PR:**
```
Title: Add community seed node - Chicago

Description:
Adding seed node at: seed-chicago.example.com:8080
- Location: Chicago, USA
- Uptime commitment: 99%
- Contact: @username (for downtime alerts)
- Running on: DigitalOcean VPS
```

**Option 2: GitHub Issue**

If you're not comfortable with PRs, open an issue with your seed node details and a maintainer will add it.

### Removing Your Seed Node

Submit a PR removing your line from `SEED-NODES.txt`, or open an issue requesting removal.

### Docker Example

```yaml
# docker-compose.yml for seed node
version: '3'
services:
  stellar-mesh-seed:
    image: stellar-mesh:latest
    ports:
      - "8080:8080"
    command: -name "Community Seed Node" -address "0.0.0.0:8080"
    restart: always
```

## Benefits of Running a Seed Node

1. **Help the network grow** - Make it easy for new members to join
2. **Community recognition** - Seed nodes are listed in the codebase
3. **Higher connectivity** - More peers will discover you
4. **Network health** - Contribute to mesh resilience

## Seed Node vs Regular Node

| Feature | Seed Node | Regular Node |
|---------|-----------|--------------|
| **Purpose** | Help others discover network | Participate in mesh |
| **Public IP** | Required | Optional |
| **Uptime** | High (95%+) | Any |
| **Incoming connections** | Many | Few |
| **Special privileges** | None | None |
| **Data stored** | Same as any node | Same as any node |

**Important:** Seed nodes have NO special authority. They don't control the network, they just help with initial discovery.

## User Experience

### Automatic Discovery (Default)

```bash
# Just works! Fetches seed list from GitHub and discovers network
./stellar-mesh -name "Alice"

# Output:
# Fetching seed node list from GitHub...
# Loaded 5 seed nodes from GitHub
# No bootstrap peer provided, discovering network via seed nodes...
# Trying seed node: seed-chicago.example.com:8080
#   Connected to seed node: seed-chicago.example.com:8080
#   Discovered 12 peers from seed node
# Network discovery complete!
```

### Manual Bootstrap (Bypasses Seed Discovery)

```bash
# Connect directly to a known peer
./stellar-mesh -name "Alice" -bootstrap "friend-ip:8080"
```

## Privacy & Isolation

If you want to run privately without auto-discovery:

```bash
# Use -bootstrap to manually specify peers (bypasses seed nodes)
./stellar-mesh -name "Private Node" -bootstrap "trusted-friend:8080"

# Or leave blank and wait for manual connections
./stellar-mesh -name "Private Node"
```

## Community Seed Nodes

Current seed nodes are listed in: [SEED-NODES.txt](SEED-NODES.txt)

To volunteer your node as a seed:
1. Ensure it meets the requirements above
2. Fork the repository
3. Add your address to `SEED-NODES.txt`
4. Submit a PR with:
   - Your node address
   - Uptime commitment
   - Optional: Contact info

Your seed will be available to new nodes as soon as the PR is merged!

## Technical Details

### Why GitHub for Seed Lists?

**Advantages:**
- ✅ No centralized server to maintain
- ✅ Community can update via PRs
- ✅ GitHub's global CDN handles traffic
- ✅ Version controlled (can see history)
- ✅ No code updates needed for new seeds
- ✅ Fallback seeds hardcoded for reliability

**Fetch Process:**
```
1. Node starts
2. HTTP GET to https://raw.githubusercontent.com/.../SEED-NODES.txt
3. Parse file (skip comments and blank lines)
4. If GitHub unreachable, use fallback seeds
5. Connect to first available seed
```

### Seed Node Discovery Flow

```go
1. Node starts, SeedNodes list checked
2. Attempt HTTP GET to /api/system on each seed
3. First responsive seed is added as peer
4. exchangePeers() protocol runs automatically
5. Seed shares list of its known peers
6. New node receives peer list
7. New node connects to discovered peers
8. (Optional) New node can disconnect from seed
9. New node is now part of the mesh!
```

### Security Considerations

- Seed nodes can't fake peer identities (cryptographic attestations)
- Seed nodes can't modify mesh data (signatures verified)
- If a seed node is malicious, worst case: gives bad peer list
- User can always manually specify `-bootstrap` to bypass seeds
- Multiple seed nodes provide redundancy

### Load Balancing

- Only first responsive seed is used
- Prevents all nodes from hammering one seed
- Seed nodes naturally balance as network grows
- Seed discovery is only needed once at startup

## FAQ

**Q: Where is the seed list stored?**
A: In `SEED-NODES.txt` in the GitHub repository. Nodes fetch it at startup.

**Q: How do I add my seed node?**
A: Submit a PR adding your address to `SEED-NODES.txt`. No code changes needed!

**Q: What if GitHub is down?**
A: Nodes fall back to hardcoded seeds in `FallbackSeedNodes` in main.go.

**Q: Do seed nodes control the network?**
A: No. They only help with initial peer discovery. Once connected, nodes operate independently.

**Q: What if all seed nodes go down?**
A: Users can use `-bootstrap` flag to manually connect, or wait for direct connections.

**Q: Can I run a seed node at home?**
A: Yes, if you have a stable IP and can port forward. Cloud VPS is recommended for reliability.

**Q: Do seed nodes store more data?**
A: No, they store the same data as any other node.

**Q: How many seed nodes does the network need?**
A: 3-5 is good for redundancy. More is better.

**Q: Can seed nodes see my private data?**
A: No, there is no private data in the mesh. Everything is cryptographically verifiable.

## Example Seed Node Setup (AWS/DigitalOcean)

```bash
# 1. Create VPS (smallest instance is fine)
# 2. Install Go and build stellar-mesh
# 3. Create systemd service

# /etc/systemd/system/stellar-mesh.service
[Unit]
Description=Stellar Mesh Seed Node
After=network.target

[Service]
Type=simple
User=stellar
ExecStart=/usr/local/bin/stellar-mesh -name "Community Seed" -address "0.0.0.0:8080"
Restart=always

[Install]
WantedBy=multi-user.target

# 4. Enable and start
sudo systemctl enable stellar-mesh
sudo systemctl start stellar-mesh

# 5. Open firewall
sudo ufw allow 8080/tcp

# 6. Verify public access
curl http://YOUR_PUBLIC_IP:8080/api/system
```

## Contributing

Want to improve seed node discovery? Ideas:

- DNS-based seed lists (like Bitcoin uses)
- Geographic distribution of seeds
- Automatic health checking
- Load balancing strategies
- Fallback mechanisms

Open an issue or PR to discuss!
