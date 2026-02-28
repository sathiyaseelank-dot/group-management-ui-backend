# firezone installation command 

    FIREZONE_ID="8a1f0d50-ab19-40a5-b769-d9aaf3ec5331" \
    FIREZONE_TOKEN=".SFMyNTY.g2gDaANtAAAAJDQzMDA0Y2FiLTQzYjYtNGUzOC1hMzU2LTg3NDRhODg0NjlmMG0AAAAkZDgzYTUwZmYtOThhNS00ZTdhLTg0YWQtY2JjOTNlMWU5ZTIwbQAAADgzRkdMTE1MMEFRRDVJQ0swMzJHUUY3S1EwTlVBSzJDRFBRUDM5MUFDN1AyRVFMSTdITE0wPT09PW4GAGa7TlCcAWIAAVGA.XznRquayUqFb_Byh6K9nbUYyLm5YST1v7i3F-KZzkNk" \
    bash <(curl -fsSL https://raw.githubusercontent.com/firezone/firezone/main/scripts/gateway-systemd-install.sh)

# twingate installation command

    curl "https://binaries.twingate.com/connector/setup.sh" | sudo 
    TWINGATE_ACCESS_TOKEN=""
    TWINGATE_REFRESH_TOKEN="" 
    TWINGATE_NETWORK="hellothere" 
    TWINGATE_LABEL_DEPLOYED_BY="linux" bash

# grpcconnector2

    curl -fsSL https://raw.githubusercontent.com/sathiyaseelank-dot/grpccontroller/main/scripts/setup.sh | sudo 
    CONTROLLER_ADDR="127.0.0.1:8443" 
    CONNECTOR_ID="connector-local-01" 
    ENROLLMENT_TOKEN="56ac57dea40aacfa98cb8205fd0f23f2" 
    CONTROLLER_CA_PATH="/etc/grpcconnector2/ca.crt" bash
