import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { cn } from "@/lib/utils";

const whitespace = /\s+/;

const sizeClasses = {
	lg: { avatar: "size-14", fallback: "text-lg" },
	md: { avatar: "size-10", fallback: "text-sm" },
	sm: { avatar: "size-8", fallback: "text-xs" },
	xl: { avatar: "size-20", fallback: "text-2xl" },
} as const;

function initials(name: string): string {
	const words = name.trim().split(whitespace).filter(Boolean);
	const first = words.at(0)?.charAt(0) ?? "?";
	const last = words.length > 1 ? (words.at(-1)?.charAt(0) ?? "") : "";
	return `${first}${last}`.toUpperCase();
}

export function MemberAvatar({
	name,
	avatarUrl,
	size = "md",
	className,
}: {
	name: string;
	avatarUrl: string | null;
	size?: keyof typeof sizeClasses;
	className?: string;
}) {
	return (
		<Avatar className={cn(sizeClasses[size].avatar, className)}>
			{avatarUrl ? <AvatarImage alt="" referrerPolicy="no-referrer" src={avatarUrl} /> : null}
			<AvatarFallback className={sizeClasses[size].fallback}>{initials(name)}</AvatarFallback>
		</Avatar>
	);
}
