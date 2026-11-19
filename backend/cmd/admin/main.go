// Operator-only bootstrap for moderator accounts and verified company ownership.
package main

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"log"
	"os"
	"time"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	if len(os.Args) < 3 {
		return fmt.Errorf("usage: admin moderator USER_UUID | admin owner COMPANY_UUID USER_UUID")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, e := pgx.Connect(ctx, dsn)
	if e != nil {
		return fmt.Errorf("database connection failed")
	}
	defer conn.Close(ctx)
	tx, e := conn.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	switch os.Args[1] {
	case "moderator":
		// The previous schema may have this legacy role-escalation guard.
		if _, e = tx.Exec(ctx, "SELECT set_config('request.jwt.claim.role','service_role',true)"); e != nil {
			return e
		}
		tag, err := tx.Exec(ctx, "UPDATE profiles.profiles SET role='moderator',is_moderator=true WHERE user_id=$1", os.Args[2])
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("profile not found: sign in once first")
		}
	case "owner":
		if len(os.Args) != 4 {
			return fmt.Errorf("usage: admin owner COMPANY_UUID USER_UUID")
		}
		var id string
		if e = tx.QueryRow(ctx, "SELECT id::text FROM catalog.companies WHERE id=$1 FOR UPDATE", os.Args[2]).Scan(&id); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, "INSERT INTO profiles.company_members(company_id,user_id,member_role) VALUES($1,$2,'owner') ON CONFLICT(company_id,user_id) DO UPDATE SET member_role='owner'", os.Args[2], os.Args[3])
		if e != nil {
			return e
		}
	default:
		return fmt.Errorf("unknown command")
	}
	return tx.Commit(ctx)
}
