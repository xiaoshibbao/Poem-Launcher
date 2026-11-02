from pathlib import Path
from PIL import Image, ImageDraw, ImageOps, ImageChops
root = Path(__file__).resolve().parents[1]
source = Image.open(root / 'assets/poem-icon-source.png').convert('RGBA')
# Apply the circular silhouette at high resolution for clean small taskbar icons.
im = ImageOps.fit(source, (1024, 1024), method=Image.Resampling.LANCZOS)
mask = Image.new('L', im.size, 0)
ImageDraw.Draw(mask).ellipse((8, 8, 1015, 1015), fill=255)
im.putalpha(ImageChops.multiply(im.getchannel('A'), mask))
im = im.resize((256, 256), Image.Resampling.LANCZOS)
im.save(root / 'assets/appicon.png')
im.save(root / 'frontend/public/appicon.png')
im.save(root / 'build/windows/icon.ico', sizes=[(16,16),(24,24),(32,32),(48,48),(64,64),(128,128),(256,256)])
