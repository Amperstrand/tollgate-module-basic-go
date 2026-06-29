# TollGate Network Security: Public WiFi Isolation

## Threat Model

A TollGate operator deploys the device at home or in a small business. The
TollGate WAN port connects to an upstream router (ISP-provided or personal).

```
Internet ← ISP Router (192.168.1.1)
              ├── PC (192.168.1.10)
              ├── Printer (192.168.1.20)
              ├── NAS (192.168.1.30)
              └── TollGate WAN (192.168.1.50)
                      ├── Public WiFi: "TollGate-XXXX" (open, captive portal)
                      └── TollGate services: payment API (2121), relay (4242)
```

**Threat**: An authenticated public WiFi user (someone who paid for internet
via the captive portal) can directly access devices on the upstream network —
the operator's PC, printer, NAS, the ISP router's admin interface, and any
other device on 192.168.1.0/24.

**Impact**: Data theft, printer abuse, router configuration changes, lateral
movement to the operator's devices. This is a CRITICAL vulnerability.

## Root Cause

The TollGate's `lan` firewall zone allows forwarding to the `wan` zone. When
WAN connects to the ISP router's LAN, forwarded traffic from public WiFi users
traverses:

```
Public WiFi client → br-lan (lan zone) → FORWARD → wan zone → ISP router LAN
```

The `lan → wan` forwarding rule has no destination filtering — it allows ALL
forwarded traffic, including traffic destined for RFC 1918 private addresses
on the upstream network.

## Solution: RFC 1918 Forwarding Filter

Add firewall rules that DROP forwarded traffic from the `lan` zone to the
`wan` zone when the destination IP is in an RFC 1918 private range.

### Rules

| Rule | Source Zone | Dest Zone | Dest IP | Action |
|------|------------|-----------|---------|--------|
| Block-LAN-To-RFC1918-10 | lan | wan | 10.0.0.0/8 | DROP |
| Block-LAN-To-RFC1918-172 | lan | wan | 172.16.0.0/12 | DROP |
| Block-LAN-To-RFC1918-192 | lan | wan | 192.168.0.0/16 | DROP |
| Block-LAN-To-LinkLocal | lan | wan | 169.254.0.0/16 | DROP |

### RFC References

- **RFC 1918** (February 1996): "Address Allocation for Private Internets" —
  defines 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16 as private use addresses.
  These are used by virtually all home and business networks.

- **RFC 3927** (May 2005): "Dynamic Configuration of IPv4 Link-Local Addresses"
  — defines 169.254.0.0/16 for link-local autoconfiguration. Devices that fail
  to get a DHCP lease self-assign from this range.

### How It Works

In OpenWrt's firewall4 (nftables), explicit `rule` sections are evaluated
BEFORE `forwarding` sections. This means the DROP rules take precedence over
the general `lan → wan` forwarding ACCEPT.

```
nftables evaluation order:
  1. rule "Block-LAN-To-RFC1918-192" (DROP if dest is 192.168.0.0/16)
  2. ... other rules ...
  3. forwarding "lan → wan" (ACCEPT all remaining traffic)
```

Traffic flow after the filter:

```
Public client → 93.184.216.34 (example.com)  → ALLOWED (not RFC 1918)
Public client → 8.8.8.8 (Google DNS)         → ALLOWED (not RFC 1918)
Public client → 192.168.1.20 (printer)       → BLOCKED ✅ (RFC 1918)
Public client → 192.168.1.1 (ISP router)     → BLOCKED ✅ (RFC 1918)
Public client → 10.42.137.1 (upstream TG)    → BLOCKED ✅ (RFC 1918)
```

## Safety Analysis

### What IS NOT affected by the filter

| Traffic | Why it's safe |
|---------|--------------|
| Router's own upstream traffic (Lightning payments, firmware updates) | Uses OUTPUT chain, not FORWARD |
| DNS resolution (dnsmasq → upstream DNS) | Router does DNS forwarding itself (OUTPUT) |
| DHCP from public clients | Goes to router's dnsmasq (INPUT), not forwarded |
| TollGate API (port 2121) | Runs on router itself (INPUT chain) |
| Captive portal (port 8080) | Runs on router itself (INPUT chain) |
| Relay service (port 4242) | Runs on router itself (INPUT chain) |
| Internet access for authenticated users | Internet destinations are public IPs, not RFC 1918 |
| Upstream gateway routing | Gateway is next-hop, not destination — packet destination is the internet host |

### What IS blocked by the filter

| Traffic | Why blocking is correct |
|---------|------------------------|
| Public client → upstream printer (192.168.1.x) | Prevents unauthorized access to operator's devices |
| Public client → ISP router admin (192.168.1.1) | Prevents router hijacking |
| Public client → upstream TollGate management | Prevents lateral movement between TollGate units |
| Public client → link-local services (169.254.x.x) | Prevents access to mDNS/SSDP services on upstream |

### Edge Cases

**Upstream is another TollGate (chaining)**:
If the upstream TollGate is on 10.x.x.1 (MAC-hash derived), public clients
cannot reach its management interface. This is CORRECT — public users should
not access upstream TollGate admin. The TollGate router itself can still
communicate with the upstream (OUTPUT chain), so Lightning channels and
inter-TollGate communication work normally.

**Double NAT (WAN IP is private)**:
If the WAN IP is 192.168.1.50 (behind ISP router), forwarded traffic to
internet hosts still works because the destination is a public IP. The filter
only blocks traffic where the FINAL DESTINATION is in RFC 1918 — not traffic
that happens to traverse an RFC 1918 network.

**VPN to private endpoint**:
If a public user connects to a VPN server at 10.x.x.x, the filter blocks it.
This is GOOD — it prevents bypassing the captive portal via VPN to a known
private IP. VPN to a public IP endpoint (e.g., 203.0.113.5) still works.

## WiFi Client Isolation (NOT enabled by default)

WiFi client isolation (`isolate='1'`) prevents public WiFi clients from
communicating with each other. This is NOT enabled by default because:

- TollGate users paying for internet may expect normal LAN behavior
- File sharing, AirDrop, casting between paying clients should work
- The RFC 1918 filter already prevents the critical threat (upstream access)

Client isolation can be enabled optionally for deployments where
client-to-client communication is unwanted (e.g., public venues):

```shell
uci set wireless.default_radio0.isolate='1'
uci set wireless.default_radio1.isolate='1'
uci commit wireless
wifi reload
```

## What's NOT Addressed Yet

### IPv6 ULA Filtering

IPv6 Unique Local Addresses (ULA, fd00::/8) are the IPv6 equivalent of RFC 1918.
Public WiFi users could potentially reach upstream devices via IPv6 ULA or
link-local (fe80::/10) addresses. IPv6 filtering should be added in a future
update:

```
config rule
    option name 'Block-LAN-To-IPv6-ULA'
    option src 'lan'
    option dest 'wan'
    option dest_ip 'fd00::/8'
    option family 'ipv6'
    option target 'DROP'

config rule
    option name 'Block-LAN-To-IPv6-LinkLocal'
    option src 'lan'
    option dest 'wan'
    option dest_ip 'fe80::/10'
    option family 'ipv6'
    option target 'DROP'
```

### Separate Operator Network

Currently, the operator and public WiFi users share the same `br-lan` bridge.
A separate operator network (password-protected WiFi on `br-private`) would
provide cleaner isolation:

```
br-lan (public zone)     → captive portal, TollGate API
br-private (private zone) → operator management, SSH, LuCI
```

The upstream tollgate-module-basic-go had this feature (`setup_private_network`),
which was simplified in the Amperstrand fork. Restoring it would provide
defense-in-depth: even if the RFC 1918 filter fails, the operator's devices
are on a separate bridge.

### Upstream TollGate Detection

If the upstream IS a TollGate, specific inter-TollGate traffic (e.g., Lightning
channel on port 9735) could be allowed. Detection approach:

```shell
WAN_GW=$(ip route show default | awk '{print $3}')
if curl -sf "http://$WAN_GW:2121/" | grep -q "tollgate"; then
    # Upstream is TollGate — optionally allow specific ports
fi
```

Not currently implemented — the RFC 1918 filter blocks all upstream private
addresses by default, which is the safer choice.
