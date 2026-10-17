import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import {
	CrownIcon,
	DoorOpenIcon,
	EyeIcon,
	LayoutGridIcon,
	LogOutIcon,
	MonitorIcon,
	MoonIcon,
	PenLineIcon,
	ScrollTextIcon,
	ShieldCheckIcon,
	SunIcon,
	UsersRoundIcon,
} from "lucide-react";
import { useState } from "react";
import { adminCampaignRequestsQuery } from "@/api/queries";
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
import { type AccountMe, type Me, tableName } from "@/lib/access";
import { signOutAndLeave } from "@/lib/sign-out";
import { setThemePreference, type ThemePreference, useThemePreference } from "@/lib/theme";
import { CharacterNameSheet } from "./character-name-sheet";
import { LeaveCampaignDialog } from "./leave-campaign-dialog";

function isThemePreference(value: string): value is ThemePreference {
	return value === "device" || value === "light" || value === "dark";
}

function profileHeading(account: AccountMe, me: Me | undefined): string {
	if (!me) {
		return account.displayName;
	}
	if (me.member.characterName) {
		return me.member.characterName;
	}
	if (me.isDm) {
		return "Dungeon Master";
	}
	return "No character yet";
}

/**
 * The profile menu, inside a campaign (character name, view as player, leave) or outside every
 * campaign. Both reach every campaign, Site admin for the owner, the theme and sign-out.
 */
export function ProfileMenu({ account, me }: { account: AccountMe; me?: Me }) {
	const theme = useThemePreference();
	const [isCharacterSheetOpen, setCharacterSheetOpen] = useState(false);
	const [isLeaveOpen, setLeaveOpen] = useState(false);
	const [isSignOutEverywhereOpen, setSignOutEverywhereOpen] = useState(false);
	const name = me ? tableName(me.member) : account.displayName;
	const avatarUrl = me ? me.member.avatarUrl : account.avatarUrl;

	return (
		<>
			<DropdownMenu>
				<DropdownMenuTrigger asChild>
					<button
						aria-label={`Profile and settings for ${name}`}
						className="inline-flex size-11 items-center justify-center rounded-full outline-none transition-shadow hover:ring-2 hover:ring-gilt/40 focus-visible:ring-[3px] focus-visible:ring-ring/45"
						type="button"
					>
						<MemberAvatar avatarUrl={avatarUrl} name={name} size="sm" />
					</button>
				</DropdownMenuTrigger>
				<DropdownMenuContent align="end" className="w-72">
					<div className="flex items-center gap-3 px-2.5 pt-2 pb-3">
						<MemberAvatar avatarUrl={avatarUrl} name={name} />
						<div className="min-w-0">
							<p className="truncate font-display font-semibold text-base text-gilt-ink tracking-wide">
								{profileHeading(account, me)}
							</p>
							<p className="flex items-center gap-2 text-muted-foreground text-sm">
								<span className="truncate">
									{me ? account.displayName : `@${account.discordUsername}`}
								</span>
								{me?.isDm ? (
									<Badge variant="gilt">
										<ShieldCheckIcon />
										DM
									</Badge>
								) : null}
							</p>
						</div>
					</div>
					{me ? (
						<>
							<DropdownMenuSeparator />
							<DropdownMenuItem
								onSelect={() => {
									setCharacterSheetOpen(true);
								}}
							>
								<PenLineIcon />
								Character name
							</DropdownMenuItem>
							{me.isDm ? <ViewAsPlayerItem viewAsPlayer={me.viewAsPlayer} /> : null}
							{me.isDm ? null : (
								<DropdownMenuItem
									onSelect={() => {
										setLeaveOpen(true);
									}}
									variant="destructive"
								>
									<DoorOpenIcon />
									Leave this campaign…
								</DropdownMenuItem>
							)}
						</>
					) : null}
					<DropdownMenuSeparator />
					<DropdownMenuGroup>
						<DropdownMenuItem asChild>
							<Link to="/">
								<LayoutGridIcon />
								All campaigns
							</Link>
						</DropdownMenuItem>
						{account.isSiteOwner ? <SiteAdminItem /> : null}
						<DropdownMenuItem asChild>
							<Link to="/privacy">
								<ScrollTextIcon />
								Privacy
							</Link>
						</DropdownMenuItem>
					</DropdownMenuGroup>
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

			{me ? (
				<>
					<CharacterNameSheet
						me={me}
						onOpenChange={setCharacterSheetOpen}
						open={isCharacterSheetOpen}
					/>
					<LeaveCampaignDialog
						campaignId={me.campaign.id}
						campaignName={me.campaign.name}
						isWaiting={false}
						onOpenChange={setLeaveOpen}
						open={isLeaveOpen}
					/>
				</>
			) : null}

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

/** View as player, a session switch: it needs the campaign on screen to send it. */
function ViewAsPlayerItem({ viewAsPlayer }: { viewAsPlayer: boolean }) {
	const { setViewAsPlayer, isSwitching } = useViewAsPlayer();
	return (
		<DropdownMenuCheckboxItem
			checked={viewAsPlayer}
			disabled={isSwitching}
			onCheckedChange={(checked) => {
				setViewAsPlayer(checked);
			}}
		>
			<EyeIcon className="text-muted-foreground" />
			View as player
		</DropdownMenuCheckboxItem>
	);
}

/** The owner's way to /admin, with the open requests waiting there (read on focus, §5.2). */
function SiteAdminItem() {
	const requests = useQuery(adminCampaignRequestsQuery);
	const openCount = requests.data?.requests.length ?? 0;
	return (
		<DropdownMenuItem asChild>
			<Link to="/admin">
				<CrownIcon />
				<span className="flex-1">Site admin</span>
				{openCount > 0 ? (
					<span className="inline-flex min-w-6 items-center justify-center rounded-full bg-primary px-1.5 font-semibold text-primary-foreground text-xs leading-5">
						{openCount}
						<span className="sr-only">{openCount === 1 ? " open request" : " open requests"}</span>
					</span>
				) : null}
			</Link>
		</DropdownMenuItem>
	);
}
