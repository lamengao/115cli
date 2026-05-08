package cmd

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/mdp/qrterminal/v3"
	"github.com/spf13/cobra"
	sdk "github.com/xhofe/115-sdk-go"
	"github.com/yibing/115cli/internal/config"
)

var (
	authRefreshToken string
	authRootID       string
	loginClientID    string
	loginTimeout     time.Duration
	loginPoll        time.Duration
)

var authCmd = &cobra.Command{
	Use:   "auth [--refresh-token <token>]",
	Short: "Save 115 credentials",
	RunE: func(cmd *cobra.Command, args []string) error {
		if authRefreshToken == "" {
			return fmt.Errorf("--refresh-token is required, or use `115cli auth login --client-id <app-id>`")
		}
		cfg := &config.Config{
			RefreshToken: authRefreshToken,
			RootID:       authRootID,
		}
		if err := config.Save(configPath, cfg); err != nil {
			return err
		}
		path := configPath
		if path == "" {
			var err error
			path, err = config.DefaultPath()
			if err != nil {
				return err
			}
		}
		fmt.Printf("saved config to %s\n", path)
		return nil
	},
}

var authCookieCmd = &cobra.Command{
	Use:   "cookie <browser-cookie>",
	Short: "Save 115 browser cookies",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := &config.Config{
			Cookie: args[0],
			RootID: authRootID,
		}
		if err := config.Save(configPath, cfg); err != nil {
			return err
		}
		path := configPath
		if path == "" {
			var err error
			path, err = config.DefaultPath()
			if err != nil {
				return err
			}
		}
		fmt.Printf("saved cookie config to %s\n", path)
		return nil
	},
}

var authLoginCmd = &cobra.Command{
	Use:   "login --client-id <app-id>",
	Short: "Login with 115 Open API QR authorization",
	RunE: func(cmd *cobra.Command, args []string) error {
		if loginClientID == "" {
			return fmt.Errorf("--client-id is required; create an app at https://open.115.com first")
		}
		if loginTimeout <= 0 {
			return fmt.Errorf("--timeout must be greater than zero")
		}
		if loginPoll <= 0 {
			return fmt.Errorf("--poll-interval must be greater than zero")
		}

		ctx, cancel := context.WithTimeout(context.Background(), loginTimeout)
		defer cancel()

		client := sdk.New()
		client.SetUserAgent("115cli/0.1")
		verifier, err := randomVerifier()
		if err != nil {
			return err
		}
		device, err := client.AuthDeviceCode(ctx, loginClientID, verifier)
		if err != nil {
			return err
		}

		fmt.Fprintln(os.Stderr, "Scan this QR code with the 115 app, then confirm authorization.")
		qrterminal.GenerateWithConfig(device.QrCode, qrterminal.Config{
			Level:     qrterminal.L,
			Writer:    os.Stderr,
			BlackChar: qrterminal.BLACK,
			WhiteChar: qrterminal.WHITE,
			QuietZone: 1,
		})
		fmt.Fprintf(os.Stderr, "QR URL: %s\n", device.QrCode)

		ticker := time.NewTicker(loginPoll)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return fmt.Errorf("login timed out before QR authorization completed")
			case <-ticker.C:
				status, err := client.QrCodeStatus(ctx, device.UID, strconv.FormatInt(device.Time, 10), device.Sign)
				if err != nil {
					return err
				}
				if verbose {
					fmt.Fprintf(os.Stderr, "qr status: %d %s\n", status.Status, status.Msg)
				}
				switch status.Status {
				case 0:
					continue
				case 1:
					fmt.Fprintln(os.Stderr, "QR scanned; waiting for confirmation...")
					continue
				case 2:
					token, err := client.CodeToToken(ctx, device.UID, verifier)
					if err != nil {
						return err
					}
					cfg := &config.Config{
						RefreshToken:     token.RefreshToken,
						AccessToken:      token.AccessToken,
						ClientID:         loginClientID,
						RootID:           authRootID,
						LastTokenRefresh: time.Now(),
					}
					if err := config.Save(configPath, cfg); err != nil {
						return err
					}
					path := configPath
					if path == "" {
						var err error
						path, err = config.DefaultPath()
						if err != nil {
							return err
						}
					}
					fmt.Fprintf(os.Stderr, "login succeeded; saved config to %s\n", path)
					return nil
				case -1:
					return fmt.Errorf("QR code expired; run auth login again")
				case -2:
					return fmt.Errorf("QR authorization was canceled")
				default:
					return fmt.Errorf("unexpected QR status %d: %s", status.Status, status.Msg)
				}
			}
		}
	},
}

func init() {
	authCmd.Flags().StringVar(&authRefreshToken, "refresh-token", "", "115 Open API refresh token")
	authCmd.Flags().StringVar(&authRootID, "root-id", "0", "115 root folder id")
	authCookieCmd.Flags().StringVar(&authRootID, "root-id", "0", "115 root folder id")
	authLoginCmd.Flags().StringVar(&loginClientID, "client-id", "", "115 Open Platform App ID")
	authLoginCmd.Flags().StringVar(&authRootID, "root-id", "0", "115 root folder id")
	authLoginCmd.Flags().DurationVar(&loginTimeout, "timeout", 3*time.Minute, "QR login timeout")
	authLoginCmd.Flags().DurationVar(&loginPoll, "poll-interval", 2*time.Second, "QR status polling interval")
	authCmd.AddCommand(authCookieCmd, authLoginCmd)
}

func randomVerifier() (string, error) {
	b := make([]byte, 64)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
