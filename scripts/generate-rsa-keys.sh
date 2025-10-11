#!/bin/bash

# RSA密钥生成脚本
# 用于生成OIDC ID Token签名所需的RSA密钥对

set -e

# 默认配置
KEY_SIZE=2048
OUTPUT_DIR="./keys"
PRIVATE_KEY_FILE="oidc_private_key.pem"
PUBLIC_KEY_FILE="oidc_public_key.pem"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 打印帮助信息
print_help() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Generate RSA key pair for OIDC ID Token signing"
    echo ""
    echo "Options:"
    echo "  -s, --size SIZE      Key size in bits (default: 2048, recommended: 2048 or 4096)"
    echo "  -o, --output DIR     Output directory (default: ./keys)"
    echo "  -h, --help           Show this help message"
    echo ""
    echo "Example:"
    echo "  $0 --size 4096 --output /etc/mliev-iam/keys"
}

# 解析命令行参数
while [[ $# -gt 0 ]]; do
    case $1 in
        -s|--size)
            KEY_SIZE="$2"
            shift 2
            ;;
        -o|--output)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        -h|--help)
            print_help
            exit 0
            ;;
        *)
            echo -e "${RED}Error: Unknown option $1${NC}"
            print_help
            exit 1
            ;;
    esac
done

echo -e "${GREEN}=== OIDC RSA Key Pair Generator ===${NC}"
echo ""

# 验证密钥大小
if [ "$KEY_SIZE" -lt 2048 ]; then
    echo -e "${RED}Error: Key size must be at least 2048 bits${NC}"
    exit 1
fi

# 创建输出目录
if [ ! -d "$OUTPUT_DIR" ]; then
    echo -e "${YELLOW}Creating output directory: $OUTPUT_DIR${NC}"
    mkdir -p "$OUTPUT_DIR"
fi

# 设置文件路径
PRIVATE_KEY_PATH="$OUTPUT_DIR/$PRIVATE_KEY_FILE"
PUBLIC_KEY_PATH="$OUTPUT_DIR/$PUBLIC_KEY_FILE"

# 检查文件是否已存在
if [ -f "$PRIVATE_KEY_PATH" ] || [ -f "$PUBLIC_KEY_PATH" ]; then
    echo -e "${YELLOW}Warning: Key files already exist in $OUTPUT_DIR${NC}"
    read -p "Do you want to overwrite them? (yes/no): " -r
    echo
    if [[ ! $REPLY =~ ^[Yy]es$ ]]; then
        echo -e "${YELLOW}Operation cancelled${NC}"
        exit 0
    fi
fi

# 生成私钥
echo -e "${GREEN}Generating $KEY_SIZE-bit RSA private key...${NC}"
openssl genrsa -out "$PRIVATE_KEY_PATH" "$KEY_SIZE" 2>/dev/null

# 设置私钥权限 (仅所有者可读写)
chmod 600 "$PRIVATE_KEY_PATH"

# 从私钥导出公钥
echo -e "${GREEN}Extracting public key...${NC}"
openssl rsa -in "$PRIVATE_KEY_PATH" -pubout -out "$PUBLIC_KEY_PATH" 2>/dev/null

# 设置公钥权限 (所有人可读)
chmod 644 "$PUBLIC_KEY_PATH"

echo -e "${GREEN}=== Key Generation Complete ===${NC}"
echo ""
echo "Private Key: $PRIVATE_KEY_PATH"
echo "Public Key:  $PUBLIC_KEY_PATH"
echo ""
echo -e "${YELLOW}IMPORTANT SECURITY NOTES:${NC}"
echo "1. Keep the private key ($PRIVATE_KEY_FILE) SECURE and NEVER share it"
echo "2. Back up the private key in a secure location"
echo "3. The private key file has been set to permissions 600 (owner read/write only)"
echo "4. Consider using environment variables or a secrets manager for production"
echo ""
echo -e "${GREEN}=== Configuration Instructions ===${NC}"
echo ""
echo "Add the following to your config.yaml:"
echo ""
echo "oidc:"
echo "  issuer: \"https://your-domain.com\""
echo "  signing:"
echo "    algorithm: \"RS256\""
echo "    rsa_private_key_path: \"$PRIVATE_KEY_PATH\""
echo "    # OR use environment variable:"
echo "    # rsa_private_key_env: \"OIDC_RSA_PRIVATE_KEY\""
echo "  key_id: \"default\""
echo ""
echo "Alternatively, set environment variable:"
echo "export OIDC_RSA_PRIVATE_KEY=\"\$(cat $PRIVATE_KEY_PATH)\""
echo ""
echo -e "${GREEN}Done!${NC}"

