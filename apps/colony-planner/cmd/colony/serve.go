package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/BBruington/colony-planner/internal/app"
	"github.com/BBruington/colony-planner/internal/auth"
	"github.com/BBruington/colony-planner/internal/bot"
	"github.com/BBruington/colony-planner/internal/core"
)

func serve(commandContext context.Context) error {
	loaded, err := loadConfig()
	if err != nil {
		return err
	}
	if loaded.publicURL == "" {
		return errors.New("PUBLIC_URL is not set (run `mise run setup` to create .env)")
	}
	if err := auth.CheckBuild(loaded.appEnvironment); err != nil {
		return err
	}
	if err := auth.CheckPublicURL(loaded.publicURL, loaded.appEnvironment); err != nil {
		return err
	}
	if loaded.appEnvironment == "production" && loaded.dmDiscordID == "" {
		return errors.New("DM_DISCORD_ID is not set: without it nobody is the DM")
	}
	logger := newLogger(loaded.appEnvironment)
	slog.SetDefault(logger)

	pool, err := openDatabase(commandContext, loaded.databaseURL, serverApplicationName)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := migrateUp(commandContext, pool, logger); err != nil {
		return err
	}

	dmDiscordID := auth.EffectiveDmDiscordID(loaded.dmDiscordID)
	// Without DISCORD_BOT_TOKEN, bot.New returns a Beny Bot that is off: its pings do nothing and Run only
	// waits. A token without DISCORD_GUILD_ID is refused here rather than ignored.
	benyBot, err := bot.New(bot.Options{
		Token:       loaded.discordBotToken,
		GuildID:     loaded.discordGuildID,
		DmDiscordID: dmDiscordID,
		PublicURL:   loaded.publicURL,
		Logger:      logger.With(slog.String("component", "beny_bot")),
	})
	if err != nil {
		return err
	}

	// Core evicts from the cache that auth reads, so one cache is shared by both.
	sessionCache := auth.NewSessionCache()
	service := core.New(core.Options{
		Pool:         pool,
		Notifier:     benyBot,
		Logger:       logger,
		DmDiscordID:  dmDiscordID,
		Sessions:     sessionCache,
		BotInviteURL: bot.InviteURL(loaded.discordClientID),
	})
	authService, err := auth.New(auth.Options{
		Core:                service,
		Logger:              logger,
		SessionCache:        sessionCache,
		PublicURL:           loaded.publicURL,
		AppEnvironment:      loaded.appEnvironment,
		DiscordClientID:     loaded.discordClientID,
		DiscordClientSecret: loaded.discordClientSecret,
		DmDiscordID:         dmDiscordID,
	})
	if err != nil {
		return err
	}
	server, err := app.New(app.Options{Core: service, Auth: authService, Logger: logger})
	if err != nil {
		return err
	}

	// Bind before announcing anything, so a port already in use is the only message, not a claim that
	// the server is listening followed by the error.
	listener, err := net.Listen("tcp", loaded.listenAddress())
	if err != nil {
		return err
	}

	group, groupContext := errgroup.WithContext(commandContext)
	httpServer := &http.Server{
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		// Request contexts end at shutdown, which closes open SSE streams instead of waiting on them.
		BaseContext: func(net.Listener) context.Context { return groupContext },
	}
	group.Go(func() error {
		logger.Info("Colony Planner is listening",
			slog.String("address", "http://"+listener.Addr().String()),
			slog.String("public_url", loaded.publicURL),
			slog.Bool("dev_sign_in", auth.DevAuthEnabled),
			slog.Bool("discord_sign_in", authService.DiscordEnabled()),
			slog.Bool("beny_bot", benyBot.Enabled()),
		)
		if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})
	group.Go(func() error {
		<-groupContext.Done()
		logger.Info("shutting down")
		shutdownContext, cancel := context.WithTimeout(context.WithoutCancel(groupContext), 5*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownContext)
	})
	if benyBot.Enabled() {
		group.Go(func() error {
			// Discord is best-effort: a bot failure is logged and the website keeps serving.
			if err := benyBot.Run(groupContext, service); err != nil {
				logger.Error("Beny Bot stopped", slog.Any("error", err))
			}
			return nil
		})
	}
	return group.Wait()
}
