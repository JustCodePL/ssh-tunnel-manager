# Dev copy of the project site: the same docs/ that GitHub Pages serves, kept out
# of search engines and without analytics.
FROM nginx:alpine

COPY docs /usr/share/nginx/html
COPY deploy/nginx.conf /etc/nginx/conf.d/default.conf

RUN find /usr/share/nginx/html -name '*.html' -exec sed -i \
      -e '/statystyki\.justcode\.ac/d' \
      -e 's|<head>|<head><meta name="robots" content="noindex" />|' {} + \
 && printf 'User-agent: *\nDisallow: /\n' > /usr/share/nginx/html/robots.txt

EXPOSE 80
