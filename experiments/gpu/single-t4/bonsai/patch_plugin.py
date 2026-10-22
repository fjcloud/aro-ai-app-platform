"""Apply the experimental T4 backend to a separate prism_ternary install."""

import argparse
from pathlib import Path
import shutil

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("site", type=Path, help="directory used by pip install --target")
args = parser.parse_args()
package = args.site / "prism_ternary"
quant = package / "quant.py"
source = quant.read_text()
if source.count("return 80") != 1:
    raise SystemExit(
        "Unexpected upstream quant.py; use the pinned plugin revision in README.md"
    )
shutil.copyfile(Path(__file__).with_name("kernels_t4.py"), package / "kernels.py")
quant.write_text(source.replace("return 80", "return 75"))
print("Installed experimental SM75 kernels; use --dtype half and disable MTP.")
