#! /bin/sh

# generate_man_page_date.sh
# Copyright (C) 2021  Erik Auerswald <auerswal@unix-ag.uni-kl.de>
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program.  If not, see <http://www.gnu.org/licenses/>.

if type 'git' >/dev/null 2>&1 && test -d '.git'; then
  git log -n1 --date=short 'section.1.in' | awk '/^Date:/ { print $2 }'
  exit
elif test -f 'section.1' && test -r 'section.1'; then
  awk 'NR == 1 { gsub(/\"/, "", $4); print $4; exit }' 'section.1'
else
  date -r 'section.1.in' +%Y-%m-%d
fi