// enrolluser prints all artifacts needed to create a login user with TOTP
// already enabled, for manually inserting rows into the auth tables.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/mdp/qrterminal/v3"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"

	"interface-load-test/internal/auth"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/enrolluser <username> <password>")
		os.Exit(1)
	}

	username := os.Args[1]
	password := os.Args[2]

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		exitWithError(err)
	}

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      auth.TOTPIssuer,
		AccountName: username,
	})
	if err != nil {
		exitWithError(err)
	}

	backupCodes, backupCodeHashes, err := auth.GenerateBackupCodes()
	if err != nil {
		exitWithError(err)
	}

	userID := uuid.NewString()
	otpauthURL := key.URL()
	secret := key.Secret()

	fmt.Println("双因素认证二维码：")
	qrterminal.GenerateHalfBlock(otpauthURL, qrterminal.L, os.Stdout)
	fmt.Println()

	fmt.Println("otpauth URL（扫码失败时可手动导入）：")
	fmt.Println(otpauthURL)
	fmt.Println()

	fmt.Println("TOTP secret（手动输入密钥）：")
	fmt.Println(secret)
	fmt.Println()

	fmt.Println("备用码（只显示这一次，请立刻保存）：")
	for _, code := range backupCodes {
		fmt.Printf("  %s\n", code)
	}
	fmt.Println()

	fmt.Println("SQL（复制到 MySQL 执行）：")
	fmt.Print(buildSQL(userID, username, string(passwordHash), secret, backupCodeHashes))
}

func exitWithError(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func buildSQL(userID, username, passwordHash, secret string, backupCodeHashes []string) string {
	var b strings.Builder
	fmt.Fprintf(
		&b,
		"INSERT INTO `user` (id, username, password_hash, totp_secret, totp_enabled)\nVALUES (%s, %s, %s, %s, 1);\n\n",
		sqlString(userID),
		sqlString(username),
		sqlString(passwordHash),
		sqlString(secret),
	)
	b.WriteString("INSERT INTO backup_code (user_id, code_hash) VALUES\n")
	for i, hash := range backupCodeHashes {
		suffix := ","
		if i == len(backupCodeHashes)-1 {
			suffix = ";"
		}
		fmt.Fprintf(&b, "  (%s, %s)%s\n", sqlString(userID), sqlString(hash), suffix)
	}
	return b.String()
}

func sqlString(value string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"\x00", "\\0",
		"'", "\\'",
		"\n", "\\n",
		"\r", "\\r",
		"\x1a", "\\Z",
	)
	return "'" + replacer.Replace(value) + "'"
}
