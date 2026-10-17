// The only module that reads import.meta.env; everything else imports from here.
export const environment = {
	isDevelopment: import.meta.env.DEV,
} as const;
