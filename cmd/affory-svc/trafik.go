package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"slices"
	"strings"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/katalog"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

func (s *Sluzhba) perepodklyuchit(ctx context.Context) error {
	s.mu.Lock()
	active, expected := s.portClash != 0, s.pokolenieP+1
	s.mu.Unlock()
	if !active {
		return nil
	}
	s.Disconnect()
	return s.connect(ctx, &expected)
}

func otpechatokPravil(p PravilaNabora) string {
	b, err := json.Marshal(p)
	if err != nil {
		return ""
	}
	return string(b)
}

func (s *Sluzhba) pravilaOzhidayut(p PravilaNabora) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.portClash != 0 && s.pravilaKonfiga != otpechatokPravil(p)
}

func estPryamoyTrafik(p protokol.PravilaTrafika) bool {
	if p.PoUmolchaniyu == protokol.TrafikPryamo {
		return true
	}
	for _, a := range p.Prilozheniya {
		if a.Marshrut == protokol.TrafikPryamo {
			return true
		}
	}
	for _, d := range p.Domeny {
		if d.Marshrut == protokol.TrafikPryamo {
			return true
		}
	}
	for _, s := range p.Servisy {
		if s.Marshrut == protokol.TrafikPryamo {
			return true
		}
	}
	return false
}

func trafikPravil(p PravilaNabora) protokol.PravilaTrafika {
	r := protokol.PravilaTrafika{PoUmolchaniyu: protokol.TrafikVPN}
	if p.Trafik != nil {
		r = *p.Trafik
	}
	r.Prilozheniya = append([]protokol.PraviloPrilozheniya{}, r.Prilozheniya...)
	r.Domeny = append([]protokol.PraviloDomena{}, r.Domeny...)
	r.Servisy = append([]protokol.PraviloServisa{}, r.Servisy...)
	for _, path := range p.Protsessy {
		if !slices.ContainsFunc(r.Prilozheniya, func(a protokol.PraviloPrilozheniya) bool { return strings.EqualFold(a.Put, path) }) {
			r.Prilozheniya = append(r.Prilozheniya, protokol.PraviloPrilozheniya{Put: path, Imya: filepath.Base(path), Marshrut: protokol.TrafikPryamo})
		}
	}
	for _, domain := range p.Domeny {
		if !slices.ContainsFunc(r.Domeny, func(d protokol.PraviloDomena) bool { return strings.EqualFold(d.Domen, domain) }) {
			r.Domeny = append(r.Domeny, protokol.PraviloDomena{Domen: domain, Marshrut: protokol.TrafikPryamo})
		}
	}
	return r
}

func marshrutGoden(r protokol.MarshrutTrafika) bool {
	return r == protokol.TrafikVPN || r == protokol.TrafikPryamo
}

func proveritTrafik(p protokol.PravilaTrafika, bylo protokol.PravilaTrafika) (protokol.PravilaTrafika, error) {
	bad := func(reason string) (protokol.PravilaTrafika, error) {
		return p, fmt.Errorf("%w: %s", errPraviloNegodno, reason)
	}
	if !marshrutGoden(p.PoUmolchaniyu) {
		return bad("неизвестный маршрут по умолчанию")
	}
	if len(p.Prilozheniya) > 256 || len(p.Domeny) > 1024 || len(p.Servisy) > 64 {
		return bad("слишком много правил")
	}
	r := protokol.PravilaTrafika{PoUmolchaniyu: p.PoUmolchaniyu,
		Prilozheniya: []protokol.PraviloPrilozheniya{}, Domeny: []protokol.PraviloDomena{}, Servisy: []protokol.PraviloServisa{}}
	apps := map[string]protokol.PraviloPrilozheniya{}
	for _, a := range p.Prilozheniya {
		if !marshrutGoden(a.Marshrut) {
			return bad("неизвестный маршрут приложения")
		}
		a.Put = strings.TrimSpace(a.Put)
		if !filepath.IsAbs(a.Put) || !strings.EqualFold(filepath.Ext(a.Put), ".exe") {
			return bad("нужен полный путь к файлу .exe")
		}
		path, err := normalizovatPut(a.Put)
		if err != nil {
			old := slices.IndexFunc(bylo.Prilozheniya, func(old protokol.PraviloPrilozheniya) bool { return strings.EqualFold(old.Put, a.Put) })
			if old < 0 {
				return bad("приложение не найдено: " + a.Put)
			}
			path = bylo.Prilozheniya[old].Put
		}
		a.Put, a.Imya = path, strings.TrimSpace(a.Imya)
		if a.Imya == "" {
			a.Imya = filepath.Base(path)
		}
		if len([]rune(a.Imya)) > 80 || strings.ContainsAny(a.Imya, "\r\n\x00") {
			return bad("недопустимое имя приложения")
		}
		key := strings.ToLower(path)
		if prev, exists := apps[key]; exists {
			if prev.Marshrut != a.Marshrut || prev.Potomki != a.Potomki {
				return bad("у приложения конфликтующие правила")
			}
			continue
		}
		apps[key] = a
		r.Prilozheniya = append(r.Prilozheniya, a)
	}
	domains := map[string]protokol.MarshrutTrafika{}
	for _, d := range p.Domeny {
		d.Domen = strings.Trim(strings.ToLower(strings.TrimSpace(d.Domen)), ".")
		if !marshrutGoden(d.Marshrut) || len(d.Domen) > 253 || !reDomen.MatchString(d.Domen) || net.ParseIP(d.Domen) != nil {
			return bad("недопустимый домен или маршрут: " + d.Domen)
		}
		for _, label := range strings.Split(d.Domen, ".") {
			if len(label) > 63 {
				return bad("слишком длинная часть домена")
			}
		}
		if prev, exists := domains[d.Domen]; exists {
			if prev != d.Marshrut {
				return bad("у домена конфликтующие правила")
			}
			continue
		}
		domains[d.Domen] = d.Marshrut
		r.Domeny = append(r.Domeny, d)
	}
	services := map[string]protokol.MarshrutTrafika{}
	for _, service := range p.Servisy {
		if !marshrutGoden(service.Marshrut) {
			return bad("неизвестный маршрут сервиса")
		}
		if _, err := katalog.Domeny(service.Id); err != nil {
			return bad(err.Error())
		}
		if prev, exists := services[service.Id]; exists {
			if prev != service.Marshrut {
				return bad("у сервиса конфликтующие правила")
			}
			continue
		}
		services[service.Id] = service.Marshrut
		r.Servisy = append(r.Servisy, service)
	}
	return r, nil
}
