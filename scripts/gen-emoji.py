#!/usr/bin/env python3
"""gen-emoji.py — generate internal/md/emoji_table.go (emoji shortcodes).

The table is written from Unicode character NAMES, resolved with
unicodedata.lookup, so a mistyped codepoint cannot slip in: an unknown name
fails the build. Shortcodes follow GitHub's (gemoji) names.

    python3 -I scripts/gen-emoji.py | gofmt > internal/md/emoji_table.go

`make scripts-test` checks that the committed file matches this output. The
script is a development tool: it is never part of the kmd binary.

A single character whose East Asian Width is not "W" (text presentation by
default, e.g. WARNING SIGN) gets VARIATION SELECTOR-16 appended so terminals
draw the emoji, as GitHub does. ZWJ sequences, skin tones and flags are left
out on purpose: terminals disagree on their width.
"""

import sys
import unicodedata

# shortcode[,alias…]: NAME [+ NAME…]
TABLE = """
grinning: GRINNING FACE
smiley: SMILING FACE WITH OPEN MOUTH
smile: SMILING FACE WITH OPEN MOUTH AND SMILING EYES
grin: GRINNING FACE WITH SMILING EYES
laughing,satisfied: SMILING FACE WITH OPEN MOUTH AND TIGHTLY-CLOSED EYES
sweat_smile: SMILING FACE WITH OPEN MOUTH AND COLD SWEAT
rofl: ROLLING ON THE FLOOR LAUGHING
joy: FACE WITH TEARS OF JOY
slightly_smiling_face: SLIGHTLY SMILING FACE
upside_down_face: UPSIDE-DOWN FACE
wink: WINKING FACE
blush: SMILING FACE WITH SMILING EYES
innocent: SMILING FACE WITH HALO
smiling_face_with_three_hearts: SMILING FACE WITH SMILING EYES AND THREE HEARTS
heart_eyes: SMILING FACE WITH HEART-SHAPED EYES
star_struck: GRINNING FACE WITH STAR EYES
kissing_heart: FACE THROWING A KISS
kissing: KISSING FACE
relaxed: WHITE SMILING FACE
kissing_closed_eyes: KISSING FACE WITH CLOSED EYES
kissing_smiling_eyes: KISSING FACE WITH SMILING EYES
yum: FACE SAVOURING DELICIOUS FOOD
stuck_out_tongue: FACE WITH STUCK-OUT TONGUE
stuck_out_tongue_winking_eye: FACE WITH STUCK-OUT TONGUE AND WINKING EYE
zany_face: GRINNING FACE WITH ONE LARGE AND ONE SMALL EYE
stuck_out_tongue_closed_eyes: FACE WITH STUCK-OUT TONGUE AND TIGHTLY-CLOSED EYES
money_mouth_face: MONEY-MOUTH FACE
hugs: HUGGING FACE
hand_over_mouth: SMILING FACE WITH SMILING EYES AND HAND COVERING MOUTH
shushing_face: FACE WITH FINGER COVERING CLOSED LIPS
thinking: THINKING FACE
zipper_mouth_face: ZIPPER-MOUTH FACE
raised_eyebrow: FACE WITH ONE EYEBROW RAISED
neutral_face: NEUTRAL FACE
expressionless: EXPRESSIONLESS FACE
no_mouth: FACE WITHOUT MOUTH
smirk: SMIRKING FACE
unamused: UNAMUSED FACE
roll_eyes: FACE WITH ROLLING EYES
grimacing: GRIMACING FACE
lying_face: LYING FACE
relieved: RELIEVED FACE
pensive: PENSIVE FACE
sleepy: SLEEPY FACE
drooling_face: DROOLING FACE
sleeping: SLEEPING FACE
mask: FACE WITH MEDICAL MASK
face_with_thermometer: FACE WITH THERMOMETER
face_with_head_bandage: FACE WITH HEAD-BANDAGE
nauseated_face: NAUSEATED FACE
vomiting_face: FACE WITH OPEN MOUTH VOMITING
sneezing_face: SNEEZING FACE
hot_face: OVERHEATED FACE
cold_face: FREEZING FACE
woozy_face: FACE WITH UNEVEN EYES AND WAVY MOUTH
dizzy_face: DIZZY FACE
exploding_head: SHOCKED FACE WITH EXPLODING HEAD
cowboy_hat_face: FACE WITH COWBOY HAT
partying_face: FACE WITH PARTY HORN AND PARTY HAT
sunglasses: SMILING FACE WITH SUNGLASSES
nerd_face: NERD FACE
monocle_face: FACE WITH MONOCLE
confused: CONFUSED FACE
worried: WORRIED FACE
slightly_frowning_face: SLIGHTLY FROWNING FACE
frowning_face: WHITE FROWNING FACE
open_mouth: FACE WITH OPEN MOUTH
hushed: HUSHED FACE
astonished: ASTONISHED FACE
flushed: FLUSHED FACE
pleading_face: FACE WITH PLEADING EYES
frowning: FROWNING FACE WITH OPEN MOUTH
anguished: ANGUISHED FACE
fearful: FEARFUL FACE
cold_sweat: FACE WITH OPEN MOUTH AND COLD SWEAT
disappointed_relieved: DISAPPOINTED BUT RELIEVED FACE
cry: CRYING FACE
sob: LOUDLY CRYING FACE
scream: FACE SCREAMING IN FEAR
confounded: CONFOUNDED FACE
persevere: PERSEVERING FACE
disappointed: DISAPPOINTED FACE
sweat: FACE WITH COLD SWEAT
weary: WEARY FACE
tired_face: TIRED FACE
yawning_face: YAWNING FACE
triumph: FACE WITH LOOK OF TRIUMPH
rage,pout: POUTING FACE
angry: ANGRY FACE
cursing_face: SERIOUS FACE WITH SYMBOLS COVERING MOUTH
smiling_imp: SMILING FACE WITH HORNS
imp: IMP
skull: SKULL
skull_and_crossbones: SKULL AND CROSSBONES
hankey,poop,shit: PILE OF POO
clown_face: CLOWN FACE
japanese_ogre: JAPANESE OGRE
ghost: GHOST
alien: EXTRATERRESTRIAL ALIEN
space_invader: ALIEN MONSTER
robot: ROBOT FACE
smiley_cat: SMILING CAT FACE WITH OPEN MOUTH
smile_cat: GRINNING CAT FACE WITH SMILING EYES
joy_cat: CAT FACE WITH TEARS OF JOY
heart_eyes_cat: SMILING CAT FACE WITH HEART-SHAPED EYES
scream_cat: WEARY CAT FACE
crying_cat_face: CRYING CAT FACE
see_no_evil: SEE-NO-EVIL MONKEY
hear_no_evil: HEAR-NO-EVIL MONKEY
speak_no_evil: SPEAK-NO-EVIL MONKEY

heart: HEAVY BLACK HEART
orange_heart: ORANGE HEART
yellow_heart: YELLOW HEART
green_heart: GREEN HEART
blue_heart: BLUE HEART
purple_heart: PURPLE HEART
brown_heart: BROWN HEART
black_heart: BLACK HEART
white_heart: WHITE HEART
broken_heart: BROKEN HEART
heavy_heart_exclamation: HEAVY HEART EXCLAMATION MARK ORNAMENT
two_hearts: TWO HEARTS
revolving_hearts: REVOLVING HEARTS
heartbeat: BEATING HEART
heartpulse: GROWING HEART
sparkling_heart: SPARKLING HEART
cupid: HEART WITH ARROW
gift_heart: HEART WITH RIBBON
heart_decoration: HEART DECORATION
kiss: KISS MARK
100: HUNDRED POINTS SYMBOL
anger: ANGER SYMBOL
boom,collision: COLLISION SYMBOL
dizzy: DIZZY SYMBOL
sweat_drops: SPLASHING SWEAT SYMBOL
dash: DASH SYMBOL
hole: HOLE
speech_balloon: SPEECH BALLOON
left_speech_bubble: LEFT SPEECH BUBBLE
right_anger_bubble: RIGHT ANGER BUBBLE
thought_balloon: THOUGHT BALLOON
zzz: SLEEPING SYMBOL

wave: WAVING HAND SIGN
raised_back_of_hand: RAISED BACK OF HAND
raised_hand_with_fingers_splayed: RAISED HAND WITH FINGERS SPLAYED
hand,raised_hand: RAISED HAND
vulcan_salute: RAISED HAND WITH PART BETWEEN MIDDLE AND RING FINGERS
ok_hand: OK HAND SIGN
pinching_hand: PINCHING HAND
v: VICTORY HAND
crossed_fingers: HAND WITH INDEX AND MIDDLE FINGERS CROSSED
love_you_gesture: I LOVE YOU HAND SIGN
metal: SIGN OF THE HORNS
call_me_hand: CALL ME HAND
point_left: WHITE LEFT POINTING BACKHAND INDEX
point_right: WHITE RIGHT POINTING BACKHAND INDEX
point_up_2: WHITE UP POINTING BACKHAND INDEX
middle_finger,fu: REVERSED HAND WITH MIDDLE FINGER EXTENDED
point_down: WHITE DOWN POINTING BACKHAND INDEX
point_up: WHITE UP POINTING INDEX
+1,thumbsup: THUMBS UP SIGN
-1,thumbsdown: THUMBS DOWN SIGN
fist,fist_raised: RAISED FIST
facepunch,punch,fist_oncoming: FISTED HAND SIGN
fist_left: LEFT-FACING FIST
fist_right: RIGHT-FACING FIST
clap: CLAPPING HANDS SIGN
raised_hands: PERSON RAISING BOTH HANDS IN CELEBRATION
open_hands: OPEN HANDS SIGN
palms_up_together: PALMS UP TOGETHER
handshake: HANDSHAKE
pray: PERSON WITH FOLDED HANDS
writing_hand: WRITING HAND
nail_care: NAIL POLISH
selfie: SELFIE
muscle: FLEXED BICEPS
eyes: EYES
eye: EYE
brain: BRAIN
tongue: TONGUE
lips: MOUTH

baby: BABY
boy: BOY
girl: GIRL
man: MAN
woman: WOMAN
older_man: OLDER MAN
older_woman: OLDER WOMAN
bow: PERSON BOWING DEEPLY
raising_hand: HAPPY PERSON RAISING ONE HAND
no_good: FACE WITH NO GOOD GESTURE
ok_woman: FACE WITH OK GESTURE
person_frowning: PERSON FROWNING
shrug: SHRUG
facepalm: FACE PALM
runner,running: RUNNER
walking: PEDESTRIAN
dancer: DANCER
construction_worker: CONSTRUCTION WORKER
ninja: NINJA
santa: FATHER CHRISTMAS
superhero: SUPERHERO
bust_in_silhouette: BUST IN SILHOUETTE
busts_in_silhouette: BUSTS IN SILHOUETTE
footprints: FOOTPRINTS
children_crossing: CHILDREN CROSSING

dog: DOG FACE
cat: CAT FACE
mouse: MOUSE FACE
hamster: HAMSTER FACE
rabbit: RABBIT FACE
fox_face: FOX FACE
bear: BEAR FACE
panda_face: PANDA FACE
koala: KOALA
tiger: TIGER FACE
lion: LION FACE
cow: COW FACE
pig: PIG FACE
frog: FROG FACE
monkey_face: MONKEY FACE
monkey: MONKEY
chicken: CHICKEN
penguin: PENGUIN
bird: BIRD
baby_chick: BABY CHICK
eagle: EAGLE
duck: DUCK
owl: OWL
bat: BAT
wolf: WOLF FACE
horse: HORSE FACE
unicorn: UNICORN FACE
bee,honeybee: HONEYBEE
bug: BUG
butterfly: BUTTERFLY
snail: SNAIL
lady_beetle: LADY BEETLE
ant: ANT
spider: SPIDER
spider_web: SPIDER WEB
turtle: TURTLE
snake: SNAKE
lizard: LIZARD
dragon: DRAGON
dragon_face: DRAGON FACE
sauropod: SAUROPOD
t-rex: T-REX
whale: SPOUTING WHALE
whale2: WHALE
dolphin: DOLPHIN
fish: FISH
tropical_fish: TROPICAL FISH
blowfish: BLOWFISH
shark: SHARK
octopus: OCTOPUS
crab: CRAB
shell: SPIRAL SHELL
cactus: CACTUS
christmas_tree: CHRISTMAS TREE
evergreen_tree: EVERGREEN TREE
deciduous_tree: DECIDUOUS TREE
palm_tree: PALM TREE
seedling: SEEDLING
herb: HERB
shamrock: SHAMROCK
four_leaf_clover: FOUR LEAF CLOVER
maple_leaf: MAPLE LEAF
fallen_leaf: FALLEN LEAF
leaves: LEAF FLUTTERING IN WIND
mushroom: MUSHROOM
bouquet: BOUQUET
cherry_blossom: CHERRY BLOSSOM
white_flower: WHITE FLOWER
rose: ROSE
hibiscus: HIBISCUS
sunflower: SUNFLOWER
blossom: BLOSSOM
tulip: TULIP
earth_africa: EARTH GLOBE EUROPE-AFRICA
earth_americas: EARTH GLOBE AMERICAS
earth_asia: EARTH GLOBE ASIA-AUSTRALIA
globe_with_meridians: GLOBE WITH MERIDIANS
new_moon: NEW MOON SYMBOL
full_moon: FULL MOON SYMBOL
crescent_moon: CRESCENT MOON
sun_with_face: SUN WITH FACE
sunny: BLACK SUN WITH RAYS
star: WHITE MEDIUM STAR
star2: GLOWING STAR
stars: SHOOTING STAR
sparkles: SPARKLES
cloud: CLOUD
partly_sunny: SUN BEHIND CLOUD
cloud_with_rain: CLOUD WITH RAIN
cloud_with_lightning_and_rain: THUNDER CLOUD AND RAIN
tornado: CLOUD WITH TORNADO
fog: FOG
snowflake: SNOWFLAKE
snowman: SNOWMAN WITHOUT SNOW
zap: HIGH VOLTAGE SIGN
fire: FIRE
droplet: DROPLET
ocean: WATER WAVE
rainbow: RAINBOW
umbrella: UMBRELLA WITH RAIN DROPS
cyclone: CYCLONE
comet: COMET
milky_way: MILKY WAY

apple: RED APPLE
green_apple: GREEN APPLE
pear: PEAR
tangerine: TANGERINE
lemon: LEMON
banana: BANANA
watermelon: WATERMELON
grapes: GRAPES
strawberry: STRAWBERRY
cherries: CHERRIES
peach: PEACH
pineapple: PINEAPPLE
coconut: COCONUT
kiwi_fruit: KIWIFRUIT
tomato: TOMATO
eggplant: AUBERGINE
avocado: AVOCADO
hot_pepper: HOT PEPPER
corn: EAR OF MAIZE
carrot: CARROT
broccoli: BROCCOLI
potato: POTATO
bread: BREAD
croissant: CROISSANT
cheese: CHEESE WEDGE
egg: EGG
fried_egg: COOKING
bacon: BACON
hamburger: HAMBURGER
fries: FRENCH FRIES
pizza: SLICE OF PIZZA
hotdog: HOT DOG
taco: TACO
burrito: BURRITO
popcorn: POPCORN
salt: SALT SHAKER
bento: BENTO BOX
sushi: SUSHI
ramen: STEAMING BOWL
spaghetti: SPAGHETTI
rice: COOKED RICE
curry: CURRY AND RICE
cookie: COOKIE
cake: SHORTCAKE
birthday: BIRTHDAY CAKE
cupcake: CUPCAKE
doughnut: DOUGHNUT
chocolate_bar: CHOCOLATE BAR
candy: CANDY
lollipop: LOLLIPOP
icecream: SOFT ICE CREAM
ice_cream: ICE CREAM
coffee: HOT BEVERAGE
tea: TEACUP WITHOUT HANDLE
milk_glass: GLASS OF MILK
beer: BEER MUG
beers: CLINKING BEER MUGS
wine_glass: WINE GLASS
cocktail: COCKTAIL GLASS
tropical_drink: TROPICAL DRINK
champagne: BOTTLE WITH POPPING CORK
clinking_glasses: CLINKING GLASSES
cup_with_straw: CUP WITH STRAW
fork_and_knife: FORK AND KNIFE
plate_with_cutlery: FORK AND KNIFE WITH PLATE

soccer: SOCCER BALL
basketball: BASKETBALL AND HOOP
football: AMERICAN FOOTBALL
baseball: BASEBALL
tennis: TENNIS RACQUET AND BALL
volleyball: VOLLEYBALL
bowling: BOWLING
8ball: BILLIARDS
golf: FLAG IN HOLE
goal_net: GOAL NET
ski: SKI AND SKI BOOT
trophy: TROPHY
medal_sports: SPORTS MEDAL
1st_place_medal: FIRST PLACE MEDAL
2nd_place_medal: SECOND PLACE MEDAL
3rd_place_medal: THIRD PLACE MEDAL
dart: DIRECT HIT
game_die: GAME DIE
video_game: VIDEO GAME
joystick: JOYSTICK
jigsaw: JIGSAW PUZZLE PIECE
chess_pawn: BLACK CHESS PAWN
art: ARTIST PALETTE
performing_arts: PERFORMING ARTS
tickets: ADMISSION TICKETS
ticket: TICKET
clapper: CLAPPER BOARD
microphone: MICROPHONE
headphones: HEADPHONE
musical_note: MUSICAL NOTE
notes: MULTIPLE MUSICAL NOTES
guitar: GUITAR
musical_keyboard: MUSICAL KEYBOARD
trumpet: TRUMPET
violin: VIOLIN
drum: DRUM WITH DRUMSTICKS
tada: PARTY POPPER
confetti_ball: CONFETTI BALL
balloon: BALLOON
gift: WRAPPED PRESENT
ribbon: RIBBON
jack_o_lantern: JACK-O-LANTERN
fireworks: FIREWORKS
sparkler: FIREWORK SPARKLER
crystal_ball: CRYSTAL BALL
magic_wand: MAGIC WAND

car,red_car: AUTOMOBILE
taxi: TAXI
bus: BUS
ambulance: AMBULANCE
fire_engine: FIRE ENGINE
police_car: POLICE CAR
truck: DELIVERY TRUCK
tractor: TRACTOR
bike: BICYCLE
motorcycle: RACING MOTORCYCLE
steam_locomotive: STEAM LOCOMOTIVE
train2: TRAIN
bullettrain_side: HIGH-SPEED TRAIN
metro: METRO
airplane: AIRPLANE
flight_departure: AIRPLANE DEPARTURE
flight_arrival: AIRPLANE ARRIVING
rocket: ROCKET
helicopter: HELICOPTER
artificial_satellite: SATELLITE
satellite: SATELLITE ANTENNA
boat,sailboat: SAILBOAT
ship: SHIP
anchor: ANCHOR
construction: CONSTRUCTION SIGN
building_construction: BUILDING CONSTRUCTION
fuelpump: FUEL PUMP
rotating_light: POLICE CARS REVOLVING LIGHT
traffic_light: HORIZONTAL TRAFFIC LIGHT
vertical_traffic_light: VERTICAL TRAFFIC LIGHT
stop_sign: OCTAGONAL SIGN
passport_control: PASSPORT CONTROL
world_map: WORLD MAP
mountain: MOUNTAIN
mount_fuji: MOUNT FUJI
volcano: VOLCANO
camping: CAMPING
beach_umbrella: BEACH WITH UMBRELLA
desert_island: DESERT ISLAND
house: HOUSE BUILDING
house_with_garden: HOUSE WITH GARDEN
office: OFFICE BUILDING
hospital: HOSPITAL
bank: BANK
hotel: HOTEL
school: SCHOOL
factory: FACTORY
european_castle: EUROPEAN CASTLE
tokyo_tower: TOKYO TOWER
statue_of_liberty: STATUE OF LIBERTY
church: CHURCH
tent: TENT
bridge_at_night: BRIDGE AT NIGHT
city_sunset: CITYSCAPE AT DUSK
sunrise: SUNRISE
moyai: MOYAI
hourglass: HOURGLASS
hourglass_flowing_sand: HOURGLASS WITH FLOWING SAND
watch: WATCH
alarm_clock: ALARM CLOCK
stopwatch: STOPWATCH
timer_clock: TIMER CLOCK
calendar: TEAR-OFF CALENDAR
date: CALENDAR
spiral_calendar: SPIRAL CALENDAR PAD

iphone: MOBILE PHONE
computer: PERSONAL COMPUTER
desktop_computer: DESKTOP COMPUTER
keyboard: KEYBOARD
computer_mouse: THREE BUTTON MOUSE
printer: PRINTER
floppy_disk: FLOPPY DISK
cd: OPTICAL DISC
dvd: DVD
minidisc: MINIDISC
camera: CAMERA
camera_flash: CAMERA WITH FLASH
video_camera: VIDEO CAMERA
movie_camera: MOVIE CAMERA
tv: TELEVISION
radio: RADIO
telephone_receiver: TELEPHONE RECEIVER
phone,telephone: BLACK TELEPHONE
pager: PAGER
fax: FAX MACHINE
battery: BATTERY
electric_plug: ELECTRIC PLUG
bulb: ELECTRIC LIGHT BULB
flashlight: ELECTRIC TORCH
candle: CANDLE
moneybag: MONEY BAG
dollar: BANKNOTE WITH DOLLAR SIGN
euro: BANKNOTE WITH EURO SIGN
yen: BANKNOTE WITH YEN SIGN
pound: BANKNOTE WITH POUND SIGN
money_with_wings: MONEY WITH WINGS
credit_card: CREDIT CARD
receipt: RECEIPT
gem: GEM STONE
balance_scale: SCALES
wrench: WRENCH
hammer: HAMMER
hammer_and_wrench: HAMMER AND WRENCH
hammer_and_pick: HAMMER AND PICK
pick: PICK
nut_and_bolt: NUT AND BOLT
gear: GEAR
chains: CHAINS
toolbox: TOOLBOX
magnet: MAGNET
bricks: BRICK
link: LINK SYMBOL
paperclip: PAPERCLIP
paperclips: LINKED PAPERCLIPS
straight_ruler: STRAIGHT RULER
triangular_ruler: TRIANGULAR RULER
scissors: BLACK SCISSORS
pushpin: PUSHPIN
round_pushpin: ROUND PUSHPIN
abacus: ABACUS
compass: COMPASS
alembic: ALEMBIC
test_tube: TEST TUBE
petri_dish: PETRI DISH
dna: DNA DOUBLE HELIX
microscope: MICROSCOPE
telescope: TELESCOPE
syringe: SYRINGE
pill: PILL
adhesive_bandage: ADHESIVE BANDAGE
stethoscope: STETHOSCOPE
door: DOOR
bed: BED
toilet: TOILET
shower: SHOWER
bathtub: BATHTUB
broom: BROOM
basket: BASKET
soap: BAR OF SOAP
sponge: SPONGE
shopping_cart: SHOPPING TROLLEY
smoking: SMOKING SYMBOL
coffin: COFFIN
amphora: AMPHORA
key: KEY
old_key: OLD KEY
lock: LOCK
unlock: OPEN LOCK
lock_with_ink_pen: LOCK WITH INK PEN
closed_lock_with_key: CLOSED LOCK WITH KEY
shield: SHIELD
dagger: DAGGER KNIFE
hocho,knife: HOCHO
crossed_swords: CROSSED SWORDS
gun: PISTOL
bow_and_arrow: BOW AND ARROW
bomb: BOMB
envelope: ENVELOPE
email,e-mail: E-MAIL SYMBOL
incoming_envelope: INCOMING ENVELOPE
envelope_with_arrow: ENVELOPE WITH DOWNWARDS ARROW ABOVE
outbox_tray: OUTBOX TRAY
inbox_tray: INBOX TRAY
package: PACKAGE
mailbox: CLOSED MAILBOX WITH RAISED FLAG
postbox: POSTBOX
scroll: SCROLL
page_facing_up: PAGE FACING UP
page_with_curl: PAGE WITH CURL
bookmark_tabs: BOOKMARK TABS
bar_chart: BAR CHART
chart_with_upwards_trend: CHART WITH UPWARDS TREND
chart_with_downwards_trend: CHART WITH DOWNWARDS TREND
card_index: CARD INDEX
card_index_dividers: CARD INDEX DIVIDERS
card_file_box: CARD FILE BOX
file_cabinet: FILE CABINET
clipboard: CLIPBOARD
file_folder: FILE FOLDER
open_file_folder: OPEN FILE FOLDER
wastebasket: WASTEBASKET
newspaper: NEWSPAPER
newspaper_roll: ROLLED-UP NEWSPAPER
notebook: NOTEBOOK
notebook_with_decorative_cover: NOTEBOOK WITH DECORATIVE COVER
ledger: LEDGER
closed_book: CLOSED BOOK
green_book: GREEN BOOK
blue_book: BLUE BOOK
orange_book: ORANGE BOOK
books: BOOKS
book,open_book: OPEN BOOK
bookmark: BOOKMARK
label: LABEL
pencil2: PENCIL
memo,pencil: MEMO
black_nib: BLACK NIB
fountain_pen: LOWER LEFT FOUNTAIN PEN
pen: LOWER LEFT BALLPOINT PEN
paintbrush: LOWER LEFT PAINTBRUSH
crayon: LOWER LEFT CRAYON
mag: LEFT-POINTING MAGNIFYING GLASS
mag_right: RIGHT-POINTING MAGNIFYING GLASS
bell: BELL
no_bell: BELL WITH CANCELLATION STROKE
loudspeaker: PUBLIC ADDRESS LOUDSPEAKER
mega: CHEERING MEGAPHONE
mute: SPEAKER WITH CANCELLATION STROKE
speaker: SPEAKER
sound: SPEAKER WITH ONE SOUND WAVE
loud_sound: SPEAKER WITH THREE SOUND WAVES
crown: CROWN
tophat: TOP HAT
mortar_board: GRADUATION CAP
eyeglasses: EYEGLASSES
dark_sunglasses: DARK SUNGLASSES
necktie: NECKTIE
shirt,tshirt: T-SHIRT
jeans: JEANS
dress: DRESS
safety_vest: SAFETY VEST
handbag: HANDBAG
briefcase: BRIEFCASE
school_satchel: SCHOOL SATCHEL
luggage: LUGGAGE
ring: RING
lipstick: LIPSTICK
thread: SPOOL OF THREAD
yarn: BALL OF YARN

warning: WARNING SIGN
no_entry: NO ENTRY
no_entry_sign: NO ENTRY SIGN
x: CROSS MARK
heavy_check_mark: HEAVY CHECK MARK
white_check_mark: WHITE HEAVY CHECK MARK
ballot_box_with_check: BALLOT BOX WITH CHECK
heavy_multiplication_x: HEAVY MULTIPLICATION X
negative_squared_cross_mark: NEGATIVE SQUARED CROSS MARK
heavy_plus_sign: HEAVY PLUS SIGN
heavy_minus_sign: HEAVY MINUS SIGN
heavy_division_sign: HEAVY DIVISION SIGN
heavy_dollar_sign: HEAVY DOLLAR SIGN
currency_exchange: CURRENCY EXCHANGE
question: BLACK QUESTION MARK ORNAMENT
grey_question: WHITE QUESTION MARK ORNAMENT
exclamation,heavy_exclamation_mark: HEAVY EXCLAMATION MARK SYMBOL
grey_exclamation: WHITE EXCLAMATION MARK ORNAMENT
bangbang: DOUBLE EXCLAMATION MARK
interrobang: EXCLAMATION QUESTION MARK
o: HEAVY LARGE CIRCLE
red_circle: LARGE RED CIRCLE
orange_circle: LARGE ORANGE CIRCLE
yellow_circle: LARGE YELLOW CIRCLE
green_circle: LARGE GREEN CIRCLE
large_blue_circle: LARGE BLUE CIRCLE
purple_circle: LARGE PURPLE CIRCLE
brown_circle: LARGE BROWN CIRCLE
black_circle: MEDIUM BLACK CIRCLE
white_circle: MEDIUM WHITE CIRCLE
red_square: LARGE RED SQUARE
orange_square: LARGE ORANGE SQUARE
yellow_square: LARGE YELLOW SQUARE
green_square: LARGE GREEN SQUARE
blue_square: LARGE BLUE SQUARE
purple_square: LARGE PURPLE SQUARE
brown_square: LARGE BROWN SQUARE
black_large_square: BLACK LARGE SQUARE
white_large_square: WHITE LARGE SQUARE
large_orange_diamond: LARGE ORANGE DIAMOND
large_blue_diamond: LARGE BLUE DIAMOND
small_orange_diamond: SMALL ORANGE DIAMOND
small_blue_diamond: SMALL BLUE DIAMOND
small_red_triangle: UP-POINTING RED TRIANGLE
small_red_triangle_down: DOWN-POINTING RED TRIANGLE
diamond_shape_with_a_dot_inside: DIAMOND SHAPE WITH A DOT INSIDE
radio_button: RADIO BUTTON
information_source: INFORMATION SOURCE
abc: INPUT SYMBOL FOR LATIN LETTERS
abcd: INPUT SYMBOL FOR LATIN SMALL LETTERS
capital_abcd: INPUT SYMBOL FOR LATIN CAPITAL LETTERS
1234: INPUT SYMBOL FOR NUMBERS
symbols: INPUT SYMBOL FOR SYMBOLS
hash: NUMBER SIGN + VARIATION SELECTOR-16 + COMBINING ENCLOSING KEYCAP
asterisk: ASTERISK + VARIATION SELECTOR-16 + COMBINING ENCLOSING KEYCAP
zero: DIGIT ZERO + VARIATION SELECTOR-16 + COMBINING ENCLOSING KEYCAP
one: DIGIT ONE + VARIATION SELECTOR-16 + COMBINING ENCLOSING KEYCAP
two: DIGIT TWO + VARIATION SELECTOR-16 + COMBINING ENCLOSING KEYCAP
three: DIGIT THREE + VARIATION SELECTOR-16 + COMBINING ENCLOSING KEYCAP
four: DIGIT FOUR + VARIATION SELECTOR-16 + COMBINING ENCLOSING KEYCAP
five: DIGIT FIVE + VARIATION SELECTOR-16 + COMBINING ENCLOSING KEYCAP
six: DIGIT SIX + VARIATION SELECTOR-16 + COMBINING ENCLOSING KEYCAP
seven: DIGIT SEVEN + VARIATION SELECTOR-16 + COMBINING ENCLOSING KEYCAP
eight: DIGIT EIGHT + VARIATION SELECTOR-16 + COMBINING ENCLOSING KEYCAP
nine: DIGIT NINE + VARIATION SELECTOR-16 + COMBINING ENCLOSING KEYCAP
keycap_ten: KEYCAP TEN
new: SQUARED NEW
free: SQUARED FREE
up: SQUARED UP WITH EXCLAMATION MARK
cool: SQUARED COOL
ok: SQUARED OK
sos: SQUARED SOS
ng: SQUARED NG
id: SQUARED ID
vs: SQUARED VS
top: TOP WITH UPWARDS ARROW ABOVE
soon: SOON WITH RIGHTWARDS ARROW ABOVE
back: BACK WITH LEFTWARDS ARROW ABOVE
end: END WITH LEFTWARDS ARROW ABOVE
on: ON WITH EXCLAMATION MARK WITH LEFT RIGHT ARROW ABOVE
arrow_up: UPWARDS BLACK ARROW
arrow_down: DOWNWARDS BLACK ARROW
arrow_left: LEFTWARDS BLACK ARROW
arrow_right: BLACK RIGHTWARDS ARROW
arrow_upper_right: NORTH EAST ARROW
arrow_lower_right: SOUTH EAST ARROW
arrow_lower_left: SOUTH WEST ARROW
arrow_upper_left: NORTH WEST ARROW
arrow_up_down: UP DOWN ARROW
left_right_arrow: LEFT RIGHT ARROW
leftwards_arrow_with_hook: LEFTWARDS ARROW WITH HOOK
arrow_right_hook: RIGHTWARDS ARROW WITH HOOK
arrow_heading_up: ARROW POINTING RIGHTWARDS THEN CURVING UPWARDS
arrow_heading_down: ARROW POINTING RIGHTWARDS THEN CURVING DOWNWARDS
repeat: CLOCKWISE RIGHTWARDS AND LEFTWARDS OPEN CIRCLE ARROWS
arrows_clockwise: CLOCKWISE DOWNWARDS AND UPWARDS OPEN CIRCLE ARROWS
arrows_counterclockwise: ANTICLOCKWISE DOWNWARDS AND UPWARDS OPEN CIRCLE ARROWS
twisted_rightwards_arrows: TWISTED RIGHTWARDS ARROWS
fast_forward: BLACK RIGHT-POINTING DOUBLE TRIANGLE
rewind: BLACK LEFT-POINTING DOUBLE TRIANGLE
arrow_forward: BLACK RIGHT-POINTING TRIANGLE
arrow_backward: BLACK LEFT-POINTING TRIANGLE
pause_button: DOUBLE VERTICAL BAR
stop_button: BLACK SQUARE FOR STOP
record_button: BLACK CIRCLE FOR RECORD
loop: DOUBLE CURLY LOOP
curly_loop: CURLY LOOP
infinity: PERMANENT PAPER SIGN
copyright: COPYRIGHT SIGN
registered: REGISTERED SIGN
tm: TRADE MARK SIGN
checkered_flag: CHEQUERED FLAG
triangular_flag_on_post: TRIANGULAR FLAG ON POST
crossed_flags: CROSSED FLAGS
black_flag: WAVING BLACK FLAG
white_flag: WAVING WHITE FLAG
atom_symbol: ATOM SYMBOL
peace_symbol: PEACE SYMBOL
yin_yang: YIN YANG
radioactive: RADIOACTIVE SIGN
biohazard: BIOHAZARD SIGN
recycle: BLACK UNIVERSAL RECYCLING SYMBOL
no_smoking: NO SMOKING SYMBOL
wheelchair: WHEELCHAIR SYMBOL
beginner: JAPANESE SYMBOL FOR BEGINNER
trident: TRIDENT EMBLEM
name_badge: NAME BADGE
vibration_mode: VIBRATION MODE
mobile_phone_off: MOBILE PHONE OFF
signal_strength: ANTENNA WITH BARS
low_brightness: LOW BRIGHTNESS SYMBOL
high_brightness: HIGH BRIGHTNESS SYMBOL
part_alternation_mark: PART ALTERNATION MARK
eight_spoked_asterisk: EIGHT SPOKED ASTERISK
sparkle: SPARKLE
eight_pointed_black_star: EIGHT POINTED BLACK STAR
hotsprings: HOT SPRINGS
"""

VS16 = "️"


def emoji(spec):
    chars = "".join(unicodedata.lookup(n.strip()) for n in spec.split("+"))
    if len(chars) == 1 and unicodedata.east_asian_width(chars) != "W":
        chars += VS16
    return chars


def main():
    # every name above exists since Unicode 13; older databases cannot
    # regenerate the table (exit 3: scripts/test-scripts.sh skips the check)
    if tuple(int(x) for x in unicodedata.unidata_version.split(".")[:2]) < (13, 0):
        sys.exit(3)
    entries = {}
    for line in TABLE.splitlines():
        if not line.strip():
            continue
        codes, spec = line.split(":", 1)
        value, names = emoji(spec), spec.strip()
        for code in codes.split(","):
            code = code.strip()
            if code in entries:
                sys.exit(f"duplicate shortcode {code}")
            if not code or len(code) > 40 or any(not (c.isascii() and (c.isalnum() or c in "_+-")) for c in code):
                sys.exit(f"bad shortcode {code!r}")
            entries[code] = (value, names)
    out = [
        "// Code generated by scripts/gen-emoji.py from Unicode character names; DO NOT EDIT.",
        "",
        "package md",
        "",
        f"// emojiTable maps GitHub emoji shortcodes ({len(entries)}) to emoji, sorted by",
        "// shortcode for binary search.",
        "var emojiTable = [...]shortcode{",
    ]
    for code in sorted(entries):
        value, names = entries[code]
        lit = "".join(f"\\U{ord(c):08X}" if ord(c) > 0xFFFF else f"\\u{ord(c):04X}" if ord(c) > 0x7E else c for c in value)
        out.append(f'\t{{"{code}", "{lit}"}}, // {names}')
    out.append("}")
    print("\n".join(out))


if __name__ == "__main__":
    main()
