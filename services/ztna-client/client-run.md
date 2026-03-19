```bash
cd /home/$(whoami)/Desktop/sathi/services/ztna-client
cargo build

sudo CONTROLLER_URL="http://192.168.1.33:8081" \
ZTNA_TENANT="asd" \
CONNECTOR_TUNNEL_ADDR="192.168.1.33:9444" \
ZTNA_CLIENT_CALLBACK_BIND_ADDR="0.0.0.0" \
ZTNA_CLIENT_CALLBACK_HOST="192.168.1.40" \
ZTNA_CLIENT_PORT="19515" \
./target/debug/ztna-client ui --tenant asd
```
