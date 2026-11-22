# Fedora package for kmd. Follows the Fedora Go packaging guidelines
# (https://docs.fedoraproject.org/en-US/packaging-guidelines/Golang/).
# kmd has no Go dependencies, so no vendoring or bundled() provides are needed.
%bcond check 1

%global goipath         github.com/DipaolaFederico/kmd
Version:                0.2.0

%gometa -L -f

%global common_description %{expand:
kmd renders Markdown files as rich, colorful previews directly in the
terminal: CommonMark 0.31.2 and GitHub Flavored Markdown (tables, task lists,
alerts, footnotes), syntax highlighting, clickable hyperlinks (OSC 8), inline
images (kitty, iTerm2, sixel or Unicode blocks), Mermaid and Graphviz
diagrams, LaTeX math rendered with Unicode, and customizable color themes.
It has a built-in viewer that follows links between documents and folders,
a table of contents, live preview, a presentation mode and HTML export.
It is a single binary without dependencies that starts in a few
milliseconds.}

Name:           kmd
Release:        %autorelease
Summary:        Fast, beautiful Markdown previewer for the terminal

# The CommonMark/GFM spec examples in internal/md/testdata (CC-BY-SA-4.0)
# are only used by the test suite and are not part of the binary package.
License:        MIT
URL:            %{gourl}
# The release workflow's tarball, not GitHub's tag archive: it carries this
# version stamped into the man page and the AppStream metadata.
Source:         %{gourl}/releases/download/v%{version}/%{name}-%{version}.tar.gz

BuildRequires:  go-rpm-macros
BuildRequires:  libappstream-glib

%description %{common_description}

%prep
%goprep

%build
%global gomodulesmode GO111MODULE=on
export LDFLAGS="-X %{goipath}/internal/cli.Version=%{version}"
%gobuild -o %{gobuilddir}/bin/kmd %{goipath}
for sh in bash zsh fish; do
  %{gobuilddir}/bin/kmd --completion $sh > kmd.$sh
done

%install
install -Dpm 0755 %{gobuilddir}/bin/kmd %{buildroot}%{_bindir}/kmd
install -Dpm 0644 docs/kmd.1 %{buildroot}%{_mandir}/man1/kmd.1
install -Dpm 0644 kmd.bash %{buildroot}%{bash_completions_dir}/kmd
install -Dpm 0644 kmd.zsh  %{buildroot}%{zsh_completions_dir}/_kmd
install -Dpm 0644 kmd.fish %{buildroot}%{fish_completions_dir}/kmd.fish
install -Dpm 0644 packaging/io.github.dipaolafederico.kmd.metainfo.xml \
  %{buildroot}%{_metainfodir}/io.github.dipaolafederico.kmd.metainfo.xml

%check
appstream-util validate-relax --nonet %{buildroot}%{_metainfodir}/*.metainfo.xml
%{buildroot}%{_bindir}/kmd --version | grep -q %{version}
%if %{with check}
# the tests bound the time hostile inputs take; build machines can be slow
export KMD_TEST_TIME_SCALE=10
%gotest ./...
%endif

%files
%license LICENSE
%doc README.md
%{_bindir}/kmd
%{_mandir}/man1/kmd.1*
%{bash_completions_dir}/kmd
%{zsh_completions_dir}/_kmd
%{fish_completions_dir}/kmd.fish
%{_metainfodir}/io.github.dipaolafederico.kmd.metainfo.xml

%changelog
%autochangelog
