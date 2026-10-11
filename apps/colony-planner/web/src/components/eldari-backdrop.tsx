import { type ReactNode, useState } from "react";
import { cn } from "@/lib/utils";

/**
 * The Eldari map, darkened, behind the screens people see before they are let in (spec §6.2).
 * It stays dark in both themes: the art is the night sea either way.
 */
export function EldariBackdrop({
	children,
	className,
}: {
	children: ReactNode;
	className?: string;
}) {
	const [isArtLoaded, setArtLoaded] = useState(false);
	return (
		<div className="theme-night relative isolate min-h-dvh overflow-hidden bg-bg text-ink">
			<img
				alt=""
				className={cn(
					"absolute inset-0 -z-20 size-full scale-105 object-cover saturate-[0.85] transition-opacity duration-700",
					isArtLoaded ? "opacity-55" : "opacity-0",
				)}
				decoding="async"
				height={1080}
				onLoad={() => {
					setArtLoaded(true);
				}}
				src="/eldari.webp"
				width={1920}
			/>
			<div
				aria-hidden="true"
				className="absolute inset-0 -z-10 bg-[radial-gradient(ellipse_at_center,rgb(11_21_24/0.35)_0%,rgb(11_21_24/0.82)_62%,rgb(11_21_24/0.96)_100%)]"
			/>
			<div
				className={cn(
					"relative flex min-h-dvh flex-col items-center justify-center px-4 py-[max(2.5rem,env(safe-area-inset-top))]",
					className,
				)}
			>
				{children}
			</div>
		</div>
	);
}
