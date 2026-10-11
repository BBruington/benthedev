import {
	EyeIcon,
	LogOutIcon,
	MonitorIcon,
	MoonIcon,
	PenLineIcon,
	ShieldCheckIcon,
	SunIcon,
	UsersRoundIcon,
} from "lucide-react";
import { useState } from "react";
import { MemberAvatar } from "@/components/member-avatar";
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import {
	DropdownMenu,
	DropdownMenuCheckboxItem,
	DropdownMenuContent,
	DropdownMenuGroup,
	DropdownMenuItem,
	DropdownMenuLabel,
	DropdownMenuRadioGroup,
	DropdownMenuRadioItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useViewAsPlayer } from "@/hooks/use-view-as-player";
import { type Me, tableName } from "@/lib/access";
import { signOutAndLeave } from "@/lib/sign-out";
import { setThemePreference, type ThemePreference, useThemePreference } from "@/lib/theme";
import { CharacterNameSheet } from "./character-name-sheet";

function isThemePreference(value: string): value is ThemePreference {
	return value === "device" || value === "light" || value === "dark";
}

function profileHeading(me: Me): string {
	if (me.member.characterName) {
		return me.member.characterName;
	}
	if (me.isDm) {
		return "Dungeon Master";
	}
	return "No character yet";
}

export function ProfileMenu({ me }: { me: Me }) {
	const theme = useThemePreference();
	const { setViewAsPlayer, isSwitching } = useViewAsPlayer();
	const [isCharacterSheetOpen, setCharacterSheetOpen] = useState(false);
	const [isSignOutEverywhereOpen, setSignOutEverywhereOpen] = useState(false);
	const name = tableName(me.member);

	return (
		<>
			<DropdownMenu>
				<DropdownMenuTrigger asChild>
					<button
						aria-label={`Profile and settings for ${name}`}
						className="inline-flex size-11 items-center justify-center rounded-full outline-none transition-shadow hover:ring-2 hover:ring-gilt/40 focus-visible:ring-[3px] focus-visible:ring-ring/45"
						type="button"
					>
						<MemberAvatar avatarUrl={me.member.avatarUrl} name={name} size="sm" />
					</button>
				</DropdownMenuTrigger>
				<DropdownMenuContent align="end" className="w-72">
					<div className="flex items-center gap-3 px-2.5 pt-2 pb-3">
						<MemberAvatar avatarUrl={me.member.avatarUrl} name={name} />
						<div className="min-w-0">
							<p className="truncate font-display font-semibold text-base text-gilt-ink tracking-wide">
								{profileHeading(me)}
							</p>
							<p className="flex items-center gap-2 text-muted-foreground text-sm">
								<span className="truncate">{me.member.displayName}</span>
								{me.isDm ? (
									<Badge variant="gilt">
										<ShieldCheckIcon />
										DM
									</Badge>
								) : null}
							</p>
						</div>
					</div>
					<DropdownMenuSeparator />
					<DropdownMenuItem
						onSelect={() => {
							setCharacterSheetOpen(true);
						}}
					>
						<PenLineIcon />
						Character name
					</DropdownMenuItem>
					{me.isDm ? (
						<DropdownMenuCheckboxItem
							checked={me.viewAsPlayer}
							disabled={isSwitching}
							onCheckedChange={(checked) => {
								setViewAsPlayer(checked);
							}}
						>
							<EyeIcon className="text-muted-foreground" />
							View as player
						</DropdownMenuCheckboxItem>
					) : null}
					<DropdownMenuSeparator />
					<DropdownMenuLabel>Theme</DropdownMenuLabel>
					<DropdownMenuRadioGroup
						onValueChange={(value) => {
							if (isThemePreference(value)) {
								setThemePreference(value);
							}
						}}
						value={theme}
					>
						<DropdownMenuRadioItem value="device">
							<MonitorIcon className="text-muted-foreground" />
							Match this device
						</DropdownMenuRadioItem>
						<DropdownMenuRadioItem value="light">
							<SunIcon className="text-muted-foreground" />
							Parchment
						</DropdownMenuRadioItem>
						<DropdownMenuRadioItem value="dark">
							<MoonIcon className="text-muted-foreground" />
							Night sea
						</DropdownMenuRadioItem>
					</DropdownMenuRadioGroup>
					<DropdownMenuSeparator />
					<DropdownMenuGroup>
						<DropdownMenuItem
							onSelect={() => {
								signOutAndLeave("this-device");
							}}
						>
							<LogOutIcon />
							Sign out
						</DropdownMenuItem>
						<DropdownMenuItem
							onSelect={() => {
								setSignOutEverywhereOpen(true);
							}}
							variant="destructive"
						>
							<UsersRoundIcon />
							Sign out everywhere…
						</DropdownMenuItem>
					</DropdownMenuGroup>
				</DropdownMenuContent>
			</DropdownMenu>

			<CharacterNameSheet
				me={me}
				onOpenChange={setCharacterSheetOpen}
				open={isCharacterSheetOpen}
			/>

			<AlertDialog onOpenChange={setSignOutEverywhereOpen} open={isSignOutEverywhereOpen}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>Sign out everywhere?</AlertDialogTitle>
						<AlertDialogDescription>
							This signs you out on every phone, tablet and computer, including this one. Use it if
							you signed in somewhere you shouldn't stay signed in.
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>Stay signed in</AlertDialogCancel>
						<AlertDialogAction
							onClick={() => {
								signOutAndLeave("everywhere");
							}}
							variant="destructive"
						>
							Sign out everywhere
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</>
	);
}
